import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'package:flutter/material.dart';
import 'package:http/http.dart' as http;
import 'package:models/models.dart';

// RadioPlayer manages audio streaming from the simplex-node radio service.
//
// Lifecycle:
//   1. loadStations() fetches available stations from the API
//   2. selectStation() fetches a playlist and starts playback
//   3. Playback downloads each track via HTTP to /tmp/, then plays with gst-play-1.0
//   4. Tracks advance automatically via _nextTrackTimer
//   5. stop() kills the process and cancels all pending operations
//
// Concurrency: Uses _playSeq counter as a generation token. Any async callback
// whose generation doesn't match _playSeq is silently dropped. This prevents
// stale HTTP responses or timers from restarting playback after stop() is called.
//
// Lifecycle-aware: Pauses on app backgrounding (lifecycle paused/inactive),
// resumes on foreground, stops on detached (app exit).

/// RadioPlayer manages radioplayer functionality.
class RadioPlayer extends ChangeNotifier with WidgetsBindingObserver {
  final http.Client _httpClient;
  String _apiBase;

  // Public observable state (UI binds to these via ChangeNotifier)
  List<RadioStation> stations = [];
  RadioStation? selectedStation;
  List<RadioTrack> playlist = [];
  int currentTrackIndex = 0;
  bool loadingStations = false;
  bool loadingPlaylist = false;
  bool playing = false;
  double volume = 0.5;
  String? error;

  // Internal: process/timer management
  Process? _playbackProcess;   // Current gst-play-1.0 or gst-launch-1.0 process
  Timer? _nextTrackTimer;      // Timer for auto-advancing to next track
  bool _pausedByLifecycle = false; // True if paused due to app backgrounding
  int _playSeq = 0;            // Generation counter: increments on every stop()/selectStation()/playTrack()

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    super.didChangeAppLifecycleState(state);
    if (state == AppLifecycleState.paused || state == AppLifecycleState.inactive) {
      if (playing) {
        _pausedByLifecycle = true;
        _killProc();
        playing = false;
        notifyListeners();
      }
    } else if (state == AppLifecycleState.resumed) {
      if (_pausedByLifecycle && playlist.isNotEmpty) {
        _pausedByLifecycle = false;
        playing = true;
        _startTrackPlayback(playlist[currentTrackIndex]);
        notifyListeners();
      }
    } else if (state == AppLifecycleState.detached) {
      _pausedByLifecycle = false;
      stop();
    }
  }

  RadioPlayer({required http.Client httpClient, required String apiBase})
      : _httpClient = httpClient,
        _apiBase = apiBase {
    WidgetsBinding.instance.addObserver(this);
  }

  void updateApiBase(String apiBase) {
    _apiBase = apiBase;
    stop();
    selectedStation = null;
    playlist = [];
    currentTrackIndex = 0;
    loadingStations = false;
    loadingPlaylist = false;
    error = null;
    notifyListeners();
  }

  String get currentTrackTitle {
    if (currentTrackIndex < playlist.length) return playlist[currentTrackIndex].title;
    return '';
  }

  String get stationLabel {
    if (selectedStation != null) return '${selectedStation!.typeIcon} ${selectedStation!.name}';
    return '';
  }

  Future<void> loadStations() async {
    loadingStations = true;
    error = null;
    notifyListeners();
    try {
      final resp = await _httpClient.get(Uri.parse('$_apiBase/api/radio?action=stations'));
      if (resp.statusCode != 200) throw Exception('HTTP ${resp.statusCode}');
      final data = jsonDecode(resp.body) as Map<String, dynamic>;
      stations = (data['stations'] as List<dynamic>)
          .map((e) => RadioStation.fromJson(e as Map<String, dynamic>))
          .toList();
      loadingStations = false;
    } catch (e) {
      error = e.toString();
      loadingStations = false;
    }
    notifyListeners();
  }

  Future<void> selectStation(RadioStation station) async {
    _playSeq++;
    selectedStation = station;
    currentTrackIndex = 0;
    playing = false;
    _killProc();
    _nextTrackTimer?.cancel();
    playlist = [];
    loadingPlaylist = true;
    notifyListeners();
    try {
      final resp = await _httpClient.get(Uri.parse('$_apiBase/api/radio?action=playlist&station=${station.id}'));
      if (resp.statusCode != 200) throw Exception('HTTP ${resp.statusCode}');
      final data = jsonDecode(resp.body) as Map<String, dynamic>;
      playlist = (data['playlist'] as List<dynamic>)
          .map((e) => RadioTrack.fromJson(e as Map<String, dynamic>))
          .toList();
      loadingPlaylist = false;
    } catch (e) {
      error = e.toString();
      loadingPlaylist = false;
    }
    notifyListeners();
    if (playlist.isNotEmpty) _startTrackPlayback(playlist[0]);
  }

  Future<void> autoPlay() async {
    if (stations.isEmpty) await loadStations();
    if (stations.isNotEmpty && selectedStation == null && !playing) {
      await selectStation(stations.first);
    }
  }

  void playTrack(int index) {
    if (index >= playlist.length) return;
    _playSeq++;
    currentTrackIndex = index;
    _startTrackPlayback(playlist[index]);
    notifyListeners();
  }

  void _startTrackPlayback(RadioTrack track) {
    final seq = ++_playSeq;
    _killProc();
    final url = _trackUrl(track);
    _httpClient.get(Uri.parse(url)).then((resp) {
      if (seq != _playSeq) return;
      if (resp.statusCode != 200) {
        error = 'Track download failed: HTTP ${resp.statusCode}';
        playing = false;
        notifyListeners();
        return;
      }
      final tmpFile = File('/tmp/isle_radio_${track.title.hashCode}.mp3');
      tmpFile.writeAsBytesSync(resp.bodyBytes);
      if (seq != _playSeq) return;
      playing = true;
      notifyListeners();
      _spawnPlayback('/usr/bin/gst-play-1.0', ['-q', tmpFile.path], seq, () {
        if (seq != _playSeq) return;
        if (playing) {
          _nextTrackTimer?.cancel();
          _nextTrackTimer = Timer(const Duration(milliseconds: 500), () {
            if (seq == _playSeq) _playNext();
          });
        }
      });
    }).catchError((e) {
      if (seq != _playSeq) return;
      playing = false;
      error = 'Track download error: $e';
      notifyListeners();
    });
  }

  void togglePlayPause() {
    if (playing) {
      stop();
    } else if (playlist.isNotEmpty) {
      _startTrackPlayback(playlist[currentTrackIndex]);
    } else {
      _startFallbackStream();
    }
  }

  void _startFallbackStream() {
    final seq = ++_playSeq;
    _killProc();
    playing = true;
    error = null;
    notifyListeners();
    final streamUrl = _streamUrl();
    final proxy = _streamProxy();
    final args = <String>['-q', 'souphttpsrc', 'location=$streamUrl'];
    if (proxy.isNotEmpty) args.add('proxy=$proxy');
    args.addAll(['!', 'decodebin', '!', 'autoaudiosink']);
    _spawnPlayback('/usr/bin/gst-launch-1.0', args, seq, () {
      if (seq != _playSeq) return;
      if (playing) {
        playing = false;
        notifyListeners();
      }
    });
  }

  void _spawnPlayback(String executable, List<String> args, int seq, void Function() onExit) {
    Process.start(executable, args).then((proc) {
      if (seq != _playSeq) {
        proc.kill();
        return;
      }
      _playbackProcess = proc;
      _applyVolume();
      proc.exitCode.then((_) {
        if (seq == _playSeq && !_pendingStop()) onExit();
      });
    }).catchError((e) {
      if (seq != _playSeq) return;
      playing = false;
      error = 'Playback error: $e';
      notifyListeners();
    });
  }

  bool _pendingStop() {
    return !playing && _playSeq > 0;
  }

  String _streamUrl() {
    return '$_apiBase/api/radio/stream';
  }

  String _streamProxy() {
    if (_apiBase.contains('.onion')) return 'socks5://127.0.0.1:9050';
    return '';
  }

  void stop() {
    _playSeq++;
    playing = false;
    _killProc();
    _nextTrackTimer?.cancel();
    notifyListeners();
  }

  void skipNext() {
    if (currentTrackIndex + 1 < playlist.length) {
      currentTrackIndex++;
      _startTrackPlayback(playlist[currentTrackIndex]);
      notifyListeners();
    }
  }

  void skipPrevious() {
    if (currentTrackIndex > 0) {
      currentTrackIndex--;
      _startTrackPlayback(playlist[currentTrackIndex]);
      notifyListeners();
    }
  }

  void setVolume(double v) {
    volume = v.clamp(0.0, 1.0);
    _applyVolume();
    notifyListeners();
  }

  void resetStation() {
    stop();
    selectedStation = null;
    playlist = [];
    currentTrackIndex = 0;
    notifyListeners();
  }

  void _playNext() {
    final seq = _playSeq;
    final next = currentTrackIndex + 1;
    if (next < playlist.length) {
      currentTrackIndex = next;
      _startTrackPlayback(playlist[next]);
      notifyListeners();
    } else if (selectedStation != null) {
      _httpClient
          .get(Uri.parse('$_apiBase/api/radio?action=playlist&station=${selectedStation!.id}'))
          .then((resp) {
        if (seq != _playSeq) return;
        if (resp.statusCode != 200) return;
        final data = jsonDecode(resp.body) as Map<String, dynamic>;
        final list = (data['playlist'] as List<dynamic>)
            .map((e) => RadioTrack.fromJson(e as Map<String, dynamic>))
            .toList();
        if (seq != _playSeq) return;
        if (list.isNotEmpty) {
          playlist = list;
          currentTrackIndex = 0;
          notifyListeners();
          _startTrackPlayback(list[0]);
        }
      });
    }
  }

  void _killProc() {
    _nextTrackTimer?.cancel();
    try {
      _playbackProcess?.kill();
    } catch (_) {}
    _playbackProcess = null;
  }

  void _applyVolume() {
    Process.run('pactl', ['set-sink-volume', '@DEFAULT_SINK@', '${(volume * 100).toInt()}%']);
  }

  String _trackUrl(RadioTrack track) {
    if (track.streamUrl != null && track.streamUrl!.startsWith('/')) {
      return '$_apiBase${track.streamUrl}';
    }
    return '$_apiBase/api/radio/track?path=${selectedStation?.id ?? ""}/${track.title}';
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _playSeq++;
    _killProc();
    _nextTrackTimer?.cancel();
    super.dispose();
  }
}
