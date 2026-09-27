import 'dart:convert';
import 'dart:io';
import 'package:http/http.dart' as http;
import 'package:shared_preferences/shared_preferences.dart';

/// ServerAddress manages data model for a single server address with reachability status.
class ServerAddress {
  final String url;
  final String label;
  bool isReachable;
  DateTime? lastChecked;

  ServerAddress({
    required this.url,
    this.label = '',
    this.isReachable = false,
    this.lastChecked,
  });

/// Returns the current isOnion value.
  bool get isOnion => url.contains('.onion');

  Map<String, dynamic> toJson() => {
        'url': url,
        'label': label,
        'isReachable': isReachable,
        'lastChecked': lastChecked?.toIso8601String(),
      };

  factory ServerAddress.fromJson(Map<String, dynamic> json) => ServerAddress(
        url: json['url'] as String,
        label: json['label'] as String? ?? '',
        isReachable: json['isReachable'] as bool? ?? false,
        lastChecked: json['lastChecked'] != null ? DateTime.tryParse(json['lastChecked'] as String) : null,
      );
}

/// ServerAddressManager manages manages a list of reachable server addresses with connectivity testing.
class ServerAddressManager {
  List<ServerAddress> _addresses = [];
  static const _prefsKey = 'server_addresses';

  List<ServerAddress> get addresses => List.unmodifiable(_addresses);

  Future<void> load() async {
    final prefs = await SharedPreferences.getInstance();
    final raw = prefs.getString(_prefsKey);
    _addresses = [];
    if (raw != null && raw.isNotEmpty) {
      final list = jsonDecode(raw) as List;
      _addresses.addAll(list.map((e) => ServerAddress.fromJson(e as Map<String, dynamic>)));
    }
    for (final def in _defaultAddresses()) {
      if (!_addresses.any((a) => a.url == def.url)) {
        _addresses.add(def);
      }
    }
    await save();
  }

  List<ServerAddress> _defaultAddresses() {
    return [
      ServerAddress(url: 'http://q273p7coau3uvzeddexvdgv6andorfzvplstztheso2qcsj4yqvfzzad.onion:80',
          label: 'Dashboard (Royal Node)', isReachable: false),
    ];
  }

  Future<void> save() async {
    final prefs = await SharedPreferences.getInstance();
    final raw = jsonEncode(_addresses.map((a) => a.toJson()).toList());
    await prefs.setString(_prefsKey, raw);
  }

  void add(String url, {String? label}) {
    _addresses.add(ServerAddress(url: url, label: label ?? ''));
  }

  void removeAt(int index) {
    if (index >= 0 && index < _addresses.length) {
      _addresses.removeAt(index);
    }
  }

  String? get firstReachable {
    final reachable = _addresses.where((a) => a.isReachable).toList();
    if (reachable.isEmpty) return null;
    return reachable.first.url;
  }

  Future<bool> testAddress(int index, {http.Client? client}) async {
    if (index < 0 || index >= _addresses.length) return false;
    final addr = _addresses[index];
    final reachable = await _ping(addr.url, client: client);
    addr.isReachable = reachable;
    addr.lastChecked = DateTime.now();
    await save();
    return reachable;
  }

  Future<void> testAll({http.Client? client}) async {
    for (int i = 0; i < _addresses.length; i++) {
      await testAddress(i, client: client);
    }
  }

  Future<bool> _ping(String url, {http.Client? client}) async {
    try {
      final uri = Uri.parse(url);
      final c = client ?? http.Client();
      try {
        final res = await c.get(uri).timeout(const Duration(seconds: 5));
        return res.statusCode == 200;
      } finally {
        if (client == null) c.close();
      }
    } catch (_) {
      return false;
    }
  }

/// Returns the current firstUrl value.
  String get firstUrl => _addresses.firstWhere((a) => a.isReachable, orElse: () => _addresses.first).url;
}
