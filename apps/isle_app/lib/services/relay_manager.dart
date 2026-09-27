import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'package:http/http.dart' as http;
import 'package:shared_preferences/shared_preferences.dart';
import 'tor_aware_client.dart' show testOnionReachability;

/// RelayInfo manages data model for a SimpleX relay with SMP/XFTP addresses and quality scoring.
class RelayInfo {
  final String id;
  final String smp;
  final String xftp;
  final String ice;
  final String label;
  bool reachable;
  int latencyMs;
  DateTime? lastTested;

  RelayInfo({
    required this.id,
    required this.smp,
    required this.xftp,
    this.ice = '',
    this.label = '',
    this.reachable = false,
    this.latencyMs = 99999,
    this.lastTested,
  });

/// Returns the current hasSMP value.
  bool get hasSMP => smp.isNotEmpty;

/// Returns the current hasXFTP value.
  bool get hasXFTP => xftp.isNotEmpty;

  double get qualityScore {
    if (!reachable) return 0;
    if (latencyMs < 50) return 100;
    if (latencyMs < 100) return 90;
    if (latencyMs < 200) return 70;
    if (latencyMs < 500) return 50;
    if (latencyMs < 1000) return 30;
    return 10;
  }

  String get qualityLabel {
    if (!reachable) return 'Offline';
    if (qualityScore >= 90) return 'Excellent';
    if (qualityScore >= 70) return 'Good';
    if (qualityScore >= 50) return 'Fair';
    if (qualityScore >= 30) return 'Poor';
    return 'Very Poor';
  }

  Map<String, dynamic> toJson() => {
        'id': id,
        'smp': smp,
        'xftp': xftp,
        'ice': ice,
        'label': label,
        'reachable': reachable,
        'latencyMs': latencyMs,
        'lastTested': lastTested?.toIso8601String(),
      };

  factory RelayInfo.fromJson(Map<String, dynamic> json) => RelayInfo(
        id: json['id'] as String? ?? '',
        smp: json['smp'] as String? ?? '',
        xftp: json['xftp'] as String? ?? '',
        ice: json['ice'] as String? ?? '',
        label: json['label'] as String? ?? '',
        reachable: json['reachable'] as bool? ?? false,
        latencyMs: json['latencyMs'] as int? ?? 99999,
        lastTested: json['lastTested'] != null
            ? DateTime.tryParse(json['lastTested'] as String)
            : null,
      );
}

/// RelayManager manages manages SimpleX relay configurations with auto-testing and quality sorting.
class RelayManager {
  List<RelayInfo> _relays = [];
  static const _prefsKey = 'relay_list';
  static const _activeKey = 'active_relay_id';
  Timer? _autoTestTimer;
  static const _autoTestInterval = Duration(seconds: 60);

  List<RelayInfo> get relays => List.unmodifiable(_relays);
  List<RelayInfo> get sortedByQuality {
    final sorted = List<RelayInfo>.from(_relays);
    sorted.sort((a, b) => b.qualityScore.compareTo(a.qualityScore));
    return sorted;
  }

  Future<void> load() async {
    _relays = _defaultRelays();
    final prefs = await SharedPreferences.getInstance();
    final raw = prefs.getString(_prefsKey);
    if (raw != null && raw.isNotEmpty) {
      final list = jsonDecode(raw) as List;
      for (final e in list) {
        final cached = RelayInfo.fromJson(e as Map<String, dynamic>);
        final idx = _relays.indexWhere((r) => r.id == cached.id);
        if (idx >= 0) {
          if (cached.reachable || cached.lastTested != null) {
            _relays[idx] = cached;
          }
        } else {
          _relays.add(cached);
        }
      }
    }
    await save();
    startAutoTest();
  }

  List<RelayInfo> _defaultRelays() {
    return [
      RelayInfo(
        id: 'royal-node',
        smp: 'smp://xlxM8uqJQZgu45bi2OSDokYilqEP8RGBeBb48f0UvTY=@7czed3rxeryz4zxlo7wiwgz36yfmdwvu6ylv5wkby3trei3qsuw4lnqd.onion:5223',
        xftp: 'xftp://IROP-aDKaEDT06ShFlN36KYT2RkxzNKcDIF1x9ucTcI=@fv3pfzxih5sjf33jmusfbskmd2i3lywaaaysh6tijc7df7k6sijq3yyd.onion:443',
        ice: 'rigx5uuqk5bgvcikjfbtqenw5qn3fra34nkynrrrfp2sijophhqu4pqd.onion',
        label: 'Saint Mary Liberty Island (Royal Node)',
        reachable: false,
        latencyMs: 99999,
      ),
      RelayInfo(
        id: 'smp-relay',
        smp: 'smp://xlxM8uqJQZgu45bi2OSDokYilqEP8RGBeBb48f0UvTY=@lgienftfhdl4vwgwb2fknwokyak5drbgtix34ns3hfvtowoym5fkziad.onion:5223',
        xftp: '',
        ice: '',
        label: 'SMP Relay (dedicated)',
        reachable: false,
        latencyMs: 99999,
      ),
      RelayInfo(
        id: 'xftp-relay',
        smp: '',
        xftp: 'xftp://IROP-aDKaEDT06ShFlN36KYT2RkxzNKcDIF1x9ucTcI=@sgztkabfrn6cgw3gyvywlf2yohigbip7ow24xutmocojjbro5rfifzyd.onion:443',
        ice: '',
        label: 'XFTP Relay (dedicated)',
        reachable: false,
        latencyMs: 99999,
      ),
      RelayInfo(
        id: 'web-gateway',
        smp: '',
        xftp: '',
        ice: 'rigx5uuqk5bgvcikjfbtqenw5qn3fra34nkynrrrfp2sijophhqu4pqd.onion',
        label: 'Web Gateway (ICE)',
        reachable: false,
        latencyMs: 99999,
      ),
    ];
  }

  Future<void> save() async {
    final prefs = await SharedPreferences.getInstance();
    final raw = jsonEncode(_relays.map((r) => r.toJson()).toList());
    await prefs.setString(_prefsKey, raw);
  }

  String? get activeRelayId {
    final sorted = sortedByQuality;
    for (final r in sorted) {
      if (r.reachable) return r.id;
    }
    if (_relays.isNotEmpty) return _relays.first.id;
    return null;
  }

  RelayInfo? get activeRelay {
    final id = activeRelayId;
    if (id == null) return null;
    try {
      return _relays.firstWhere((r) => r.id == id);
    } catch (_) {
      return null;
    }
  }

  void upsertFromServer(String smp, String xftp, String ice, String onion) {
    final idx = _relays.indexWhere((r) => r.id == 'royal-node');
    if (idx >= 0) {
      _relays[idx] = RelayInfo(
        id: 'royal-node',
        smp: smp,
        xftp: xftp,
        ice: ice,
        label: _relays[idx].label,
        reachable: _relays[idx].reachable,
        latencyMs: _relays[idx].latencyMs,
        lastTested: _relays[idx].lastTested,
      );
    } else {
      _relays.insert(0, RelayInfo(
        id: 'royal-node',
        smp: smp,
        xftp: xftp,
        ice: ice,
        label: 'Saint Mary Liberty Island (Royal)',
      ));
    }
  }

  ({String host, int port})? _parseAddr(String addr, int defaultPort) {
    try {
      final uri = Uri.parse(addr);
      if (uri.host.endsWith('.onion')) {
        return (host: uri.host, port: uri.port > 0 ? uri.port : defaultPort);
      }
    } catch (_) {}
    final atMatch = RegExp(r'@([^:]+)(?::(\d+))?').firstMatch(addr);
    if (atMatch != null && atMatch.group(1)!.endsWith('.onion')) {
      return (host: atMatch.group(1)!, port: int.tryParse(atMatch.group(2) ?? '') ?? defaultPort);
    }
    return null;
  }

  Future<void> testRelay(int index, {http.Client? client}) async {
    if (index < 0 || index >= _relays.length) return;
    final relay = _relays[index];
    final stopwatch = Stopwatch()..start();

    bool smpOk = false;
    try {
      if (relay.smp.isNotEmpty) {
        final parsed = _parseAddr(relay.smp, 5223);
        if (parsed != null) {
          smpOk = await testOnionReachability(parsed.host, parsed.port, timeoutMs: 10000);
        }
      } else {
        smpOk = true;
      }
    } catch (_) {
      smpOk = false;
    }

    stopwatch.stop();
    _relays[index] = RelayInfo(
      id: relay.id,
      smp: relay.smp,
      xftp: relay.xftp,
      ice: relay.ice,
      label: relay.label,
      reachable: smpOk,
      latencyMs: stopwatch.elapsedMilliseconds,
      lastTested: DateTime.now(),
    );
    await save();
  }

  Future<void> testAll({http.Client? client}) async {
    for (int i = 0; i < _relays.length; i++) {
      await testRelay(i, client: client);
    }
  }

  void addOrUpdate(RelayInfo relay) {
    final idx = _relays.indexWhere((r) => r.id == relay.id);
    if (idx >= 0) {
      _relays[idx] = relay;
    } else {
      _relays.add(relay);
    }
  }

  void remove(String id) {
    _relays.removeWhere((r) => r.id == id);
  }

  void startAutoTest() {
    _autoTestTimer?.cancel();
    _autoTestTimer = Timer.periodic(_autoTestInterval, (_) => testAll());
    testAll();
  }

  void stopAutoTest() {
    _autoTestTimer?.cancel();
    _autoTestTimer = null;
  }

  void dispose() {
    stopAutoTest();
  }
}
