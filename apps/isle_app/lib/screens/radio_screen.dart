import 'dart:async';
import 'package:flutter/material.dart';
import 'package:api_client/api_client.dart' show SimplexApiClient;
import 'package:models/models.dart' show Identity, RadioSchedule, RadioScheduleContent, RadioStationSchedule, ScheduledBlock;

class RadioScreen extends StatefulWidget {
  final SimplexApiClient client;
  final Identity identity;

  const RadioScreen({super.key, required this.client, required this.identity});

  @override
  State<RadioScreen> createState() => _RadioScreenState();
}

class _RadioScreenState extends State<RadioScreen> with WidgetsBindingObserver {
  RadioSchedule? _schedule;
  RadioScheduleContent? _scheduleContent;
  RadioStationSchedule? _selectedStation;
  List<ScheduledBlock> _playlist = [];
  int _currentTrackIndex = 0;
  bool _loadingStations = true;
  bool _loadingPlaylist = false;
  bool _playing = false;
  double _volume = 0.5;
  String? _error;
  Timer? _nextTrackTimer;
  int _playSeq = 0;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _loadStations();
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _nextTrackTimer?.cancel();
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    super.didChangeAppLifecycleState(state);
    if (state == AppLifecycleState.paused || state == AppLifecycleState.inactive) {
      if (_playing) _pausePlayback();
    } else if (state == AppLifecycleState.resumed) {
      if (_playlist.isNotEmpty && _currentTrackIndex < _playlist.length) {
        _startPlayback(_playlist[_currentTrackIndex]);
      }
    } else if (state == AppLifecycleState.detached) {
      _stopPlayback();
    }
  }

  Future<void> _loadStations() async {
    setState(() => _loadingStations = true);
    try {
      final schedule = await widget.client.radio.schedule();
      if (mounted) {
        setState(() {
          _schedule = schedule;
          _loadingStations = false;
          _error = null;
        });
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _loadingStations = false;
          _error = e.toString();
        });
      }
    }
  }

  Future<void> _selectStation(RadioStationSchedule station) async {
    _playSeq++;
    _stopPlayback();
    setState(() {
      _selectedStation = station;
      _playlist = [];
      _currentTrackIndex = 0;
      _playing = false;
      _loadingPlaylist = true;
    });
    try {
      final content = await widget.client.radio.scheduleContent();
      if (mounted) {
        final stationSchedule = content.content[station.stationId] ?? [];
        final tracks = stationSchedule
            .where((block) => block.type == 'track' || block.type == 'music')
            .toList();
        setState(() {
          _playlist = tracks;
          _loadingPlaylist = false;
        });
        if (tracks.isNotEmpty) _startPlayback(tracks.first);
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _loadingPlaylist = false;
          _error = e.toString();
        });
      }
    }
  }

  void _startPlayback(ScheduledBlock track) {
    final seq = ++_playSeq;
    _playing = true;
    setState(() {});
    _nextTrackTimer?.cancel();
    // Estimate duration based on block length (endTime - startTime)
    final durationSec = track.endTime - track.startTime;
    if (durationSec > 0) {
      _nextTrackTimer = Timer(Duration(seconds: durationSec), () {
        if (seq != _playSeq) return;
        _playNext();
      });
    }
  }

  void _playNext() {
    if (_currentTrackIndex + 1 < _playlist.length) {
      _currentTrackIndex++;
      _startPlayback(_playlist[_currentTrackIndex]);
    } else {
      // Reload playlist for the station
      if (_selectedStation != null) _selectStation(_selectedStation!);
    }
  }

  void _pausePlayback() {
    _playing = false;
    _nextTrackTimer?.cancel();
    setState(() {});
  }

  void _stopPlayback() {
    _playSeq++;
    _playing = false;
    _nextTrackTimer?.cancel();
    setState(() {});
  }

  void _togglePlayPause() {
    if (_playing) {
      _pausePlayback();
    } else if (_playlist.isNotEmpty) {
      _startPlayback(_playlist[_currentTrackIndex]);
    }
  }

  void _skipNext() {
    if (_currentTrackIndex + 1 < _playlist.length) {
      _currentTrackIndex++;
      _startPlayback(_playlist[_currentTrackIndex]);
    }
  }

  void _skipPrevious() {
    if (_currentTrackIndex > 0) {
      _currentTrackIndex--;
      _startPlayback(_playlist[_currentTrackIndex]);
    }
  }

  void _resetStation() {
    _stopPlayback();
    setState(() {
      _selectedStation = null;
      _playlist = [];
      _currentTrackIndex = 0;
    });
  }

  ScheduledBlock? get _currentTrack => _currentTrackIndex < _playlist.length ? _playlist[_currentTrackIndex] : null;

  @override
  Widget build(BuildContext context) {
    if (_loadingStations && _schedule == null) {
      return const Scaffold(body: Center(child: CircularProgressIndicator()));
    }
    if (_error != null && _schedule == null) {
      return Scaffold(
        appBar: AppBar(title: const Text('Radio')),
        body: Center(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(Icons.radio, size: 64, color: Colors.grey),
              const SizedBox(height: 16),
              Text('Could not load stations', style: Theme.of(context).textTheme.bodyLarge),
              const SizedBox(height: 8),
              Text(_error!, style: TextStyle(color: Colors.red[700], fontSize: 12)),
              const SizedBox(height: 16),
              ElevatedButton.icon(icon: const Icon(Icons.refresh), label: const Text('Retry'), onPressed: _loadStations),
            ],
          ),
        ),
      );
    }

    if (_selectedStation != null) {
      return _buildPlayerView();
    }
    return _buildStationList();
  }

  Widget _buildStationList() {
    return Scaffold(
      appBar: AppBar(
        title: const Text('The Island Radio'),
        actions: [
          if (_loadingStations)
            const Padding(
              padding: EdgeInsets.all(16),
              child: SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2)),
            ),
          IconButton(icon: const Icon(Icons.refresh), onPressed: _loadStations),
        ],
      ),
      body: _schedule?.stations.isEmpty ?? true
          ? const Center(child: Text('No stations available'))
          : ListView.builder(
              itemCount: _schedule?.stations.length ?? 0,
              itemBuilder: (ctx, i) {
                final s = _schedule!.stations[i];
                return Card(
                  margin: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
                  child: ListTile(
                    leading: Text('${s.stationName} ${s.stationId}', style: const TextStyle(fontSize: 28)),
                    title: Text(s.stationName, style: const TextStyle(fontWeight: FontWeight.bold)),
                    subtitle: Text('${s.blocks.length} blocks', maxLines: 2, overflow: TextOverflow.ellipsis, style: TextStyle(fontSize: 12, color: Colors.grey[600])),
                    trailing: Icon(s.blocks.isNotEmpty ? Icons.play_circle_fill : Icons.stop, color: s.blocks.isNotEmpty ? Colors.green : Colors.grey, size: 32),
                    onTap: s.blocks.isNotEmpty ? () => _selectStation(s) : null,
                  ),
                );
              },
            ),
    );
  }

  Widget _buildPlayerView() {
    final s = _selectedStation!;
    final currentTrack = _currentTrack;

    return Scaffold(
      appBar: AppBar(
        title: Text('${s.stationName}'),
        leading: IconButton(icon: const Icon(Icons.arrow_back), onPressed: _resetStation),
        actions: [IconButton(icon: const Icon(Icons.refresh), onPressed: () => _selectStation(s))],
      ),
      body: Column(
        children: [
          Container(
            padding: const EdgeInsets.all(12),
            color: _playing ? Colors.green.withValues(alpha: 0.1) : Theme.of(context).colorScheme.primaryContainer,
            child: Column(
              children: [
                Icon(Icons.radio, size: 36, color: _playing ? Colors.green : Colors.grey),
                const SizedBox(height: 6),
                if (currentTrack != null) ...[
                  Text(currentTrack.title, style: Theme.of(context).textTheme.titleSmall, textAlign: TextAlign.center),
                  const SizedBox(height: 2),
                  Text(currentTrack.type, style: TextStyle(color: Colors.grey[600], fontSize: 11)),
                ] else
                  Text('Loading playlist...', style: TextStyle(color: Colors.grey[600], fontSize: 12)),
                const SizedBox(height: 8),
                Row(
                  mainAxisAlignment: MainAxisAlignment.center,
                  children: [
                    IconButton(icon: const Icon(Icons.skip_previous, size: 20), onPressed: _currentTrackIndex > 0 ? _skipPrevious : null),
                    IconButton(icon: Icon(_playing ? Icons.stop : Icons.play_arrow, size: 28), color: _playing ? Colors.red : Colors.green, onPressed: _togglePlayPause),
                    IconButton(icon: const Icon(Icons.skip_next, size: 20), onPressed: _currentTrackIndex + 1 < _playlist.length ? _skipNext : null),
                  ],
                ),
                Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 32),
                  child: Row(
                    children: [
                      const Icon(Icons.volume_down, size: 14),
                      Expanded(child: Slider(value: _volume, onChanged: (v) => setState(() => _volume = v), min: 0.0, max: 1.0, divisions: 20)),
                      const Icon(Icons.volume_up, size: 14),
                    ],
                  ),
                ),
                if (_playlist.isNotEmpty)
                  Padding(
                    padding: const EdgeInsets.only(top: 4),
                    child: Text('Track ${_currentTrackIndex + 1} of ${_playlist.length}', style: TextStyle(color: Colors.grey[600], fontSize: 10)),
                  ),
              ],
            ),
          ),
          Expanded(
            child: _loadingPlaylist
                ? const Center(child: CircularProgressIndicator())
                : _playlist.isEmpty
                    ? const Center(child: Text('No tracks — upload audio via API'))
                    : ListView.builder(
                        itemCount: _playlist.length,
                        itemBuilder: (ctx, i) {
                          final t = _playlist[i];
                          final isCurrent = i == _currentTrackIndex;
                          return Card(
                            margin: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
                            color: isCurrent ? Colors.green.withValues(alpha: 0.08) : null,
                            child: ListTile(
                              leading: Icon(_iconForTrack(t), color: isCurrent ? Colors.green : _colorForTrack(t)),
                              title: Text(t.title, maxLines: 1, overflow: TextOverflow.ellipsis, style: TextStyle(fontWeight: isCurrent ? FontWeight.bold : FontWeight.normal)),
                              subtitle: Text('${t.type}  ${t.endTime - t.startTime}s', style: TextStyle(fontSize: 11, color: Colors.grey[600])),
                              trailing: IconButton(icon: Icon(Icons.play_arrow, color: isCurrent && _playing ? Colors.green : null), tooltip: 'Play', onPressed: () => _startPlayback(t)),
                            ),
                          );
                        },
                      ),
          ),
        ],
      ),
    );
  }

  IconData _iconForTrack(ScheduledBlock t) {
    if (t.type == 'ad') return Icons.paid;
    if (t.type == 'announcement') return Icons.campaign;
    return Icons.music_note;
  }

  Color _colorForTrack(ScheduledBlock t) {
    if (t.type == 'ad') return Colors.orange;
    if (t.type == 'announcement') return Colors.blue;
    return Colors.grey;
  }
}