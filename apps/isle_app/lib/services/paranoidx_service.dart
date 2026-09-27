import 'dart:async';
import 'dart:convert';
import 'package:http/http.dart' as http;

/// ParanoidXService manages HTTP client for the ParanoidX API endpoints.
class ParanoidXService {
  final String _apiBase;
  final http.Client _httpClient;
  bool _initialized = false;

  ParanoidXService({
    String apiBase = 'http://127.0.0.1:8080',
    http.Client? httpClient,
  }) : _apiBase = apiBase,
       _httpClient = httpClient ?? http.Client();

  /// Initialize the service (placeholder for native initialization)
  Future<void> initialize() async {
    if (_initialized) return;
    // In a real app, this might initialize native components
    _initialized = true;
  }

  /// Dispose resources
  void dispose() {
    _httpClient.close();
    _initialized = false;
  }

  Future<Map<String, dynamic>> getStatus() async {
    try {
      final resp = await _httpClient
          .get(Uri.parse('$_apiBase/api/paranoidx/status'))
          .timeout(const Duration(seconds: 5));
      if (resp.statusCode == 200) {
        return jsonDecode(resp.body) as Map<String, dynamic>;
      }
    } catch (_) {}
    return {
      'overall_healthy': false,
      'layers': [
        {'layer': 'v2ray', 'healthy': false, 'message': 'unreachable'},
        {'layer': 'vpn', 'healthy': false, 'message': 'unreachable'},
        {'layer': 'tor', 'healthy': false, 'message': 'unreachable'},
        {'layer': 'simplex', 'healthy': false, 'message': 'unreachable'},
      ],
    };
  }

  Future<Map<String, dynamic>> getConfig() async {
    try {
      final resp = await _httpClient
          .get(Uri.parse('$_apiBase/api/paranoidx/config'))
          .timeout(const Duration(seconds: 5));
      if (resp.statusCode == 200) {
        return jsonDecode(resp.body) as Map<String, dynamic>;
      }
    } catch (_) {}
    return {};
  }

  Future<Map<String, dynamic>> buildChain() async {
    try {
      final resp = await _httpClient
          .post(Uri.parse('$_apiBase/api/paranoidx/chain/build'))
          .timeout(const Duration(seconds: 60));
      if (resp.statusCode == 200) {
        return jsonDecode(resp.body) as Map<String, dynamic>;
      }
    } catch (_) {}
    return {'status': 'error', 'error': 'request failed'};
  }

  Future<Map<String, dynamic>> teardownChain() async {
    try {
      final resp = await _httpClient
          .post(Uri.parse('$_apiBase/api/paranoidx/chain/teardown'))
          .timeout(const Duration(seconds: 30));
      if (resp.statusCode == 200) {
        return jsonDecode(resp.body) as Map<String, dynamic>;
      }
    } catch (_) {}
    return {'status': 'error', 'error': 'request failed'};
  }

  Future<Map<String, dynamic>> getChainState() async {
    try {
      final resp = await _httpClient
          .get(Uri.parse('$_apiBase/api/paranoidx/chain/state'))
          .timeout(const Duration(seconds: 5));
      if (resp.statusCode == 200) {
        return jsonDecode(resp.body) as Map<String, dynamic>;
      }
    } catch (_) {}
    return {'state': 'down'};
  }

  Future<Map<String, dynamic>> testChain() async {
    try {
      final resp = await _httpClient
          .get(Uri.parse('$_apiBase/api/paranoidx/chain/test'))
          .timeout(const Duration(seconds: 30));
      if (resp.statusCode == 200) {
        return jsonDecode(resp.body) as Map<String, dynamic>;
      }
    } catch (_) {}
    return {'results': {}, 'tested_at': ''};
  }

  Future<Map<String, dynamic>> getVPNProfiles() async {
    try {
      final resp = await _httpClient
          .get(Uri.parse('$_apiBase/api/paranoidx/vpn/profiles'))
          .timeout(const Duration(seconds: 5));
      if (resp.statusCode == 200) {
        return jsonDecode(resp.body) as Map<String, dynamic>;
      }
    } catch (_) {}
    return {'profiles': [], 'active': ''};
  }

  Future<Map<String, dynamic>> addVPNProfile(String name, String description, String config) async {
    try {
      final resp = await _httpClient
          .post(
            Uri.parse('$_apiBase/api/paranoidx/vpn/profiles'),
            headers: {'Content-Type': 'application/json'},
            body: jsonEncode({'name': name, 'description': description, 'config': config}),
          )
          .timeout(const Duration(seconds: 10));
      if (resp.statusCode == 200) {
        return jsonDecode(resp.body) as Map<String, dynamic>;
      }
    } catch (_) {}
    return {'status': 'error'};
  }

  Future<Map<String, dynamic>> vpnUp(String name) async {
    try {
      final resp = await _httpClient
          .post(
            Uri.parse('$_apiBase/api/paranoidx/vpn/up'),
            headers: {'Content-Type': 'application/json'},
            body: jsonEncode({'name': name}),
          )
          .timeout(const Duration(seconds: 15));
      if (resp.statusCode == 200) {
        return jsonDecode(resp.body) as Map<String, dynamic>;
      }
    } catch (_) {}
    return {'status': 'error'};
  }

  Future<Map<String, dynamic>> vpnDown(String name) async {
    try {
      final resp = await _httpClient
          .post(
            Uri.parse('$_apiBase/api/paranoidx/vpn/down'),
            headers: {'Content-Type': 'application/json'},
            body: jsonEncode({'name': name}),
          )
          .timeout(const Duration(seconds: 15));
      if (resp.statusCode == 200) {
        return jsonDecode(resp.body) as Map<String, dynamic>;
      }
    } catch (_) {}
    return {'status': 'error'};
  }

  Future<Map<String, dynamic>> vpnDelete(String name) async {
    try {
      final resp = await _httpClient
          .post(
            Uri.parse('$_apiBase/api/paranoidx/vpn/delete'),
            headers: {'Content-Type': 'application/json'},
            body: jsonEncode({'name': name}),
          )
          .timeout(const Duration(seconds: 10));
      if (resp.statusCode == 200) {
        return jsonDecode(resp.body) as Map<String, dynamic>;
      }
    } catch (_) {}
    return {'status': 'error'};
  }

  Future<Map<String, dynamic>> updateConfig({bool? v2ray, bool? vpn, bool? tor}) async {
    try {
      final body = <String, dynamic>{};
      if (v2ray != null) body['v2ray_enabled'] = v2ray;
      if (vpn != null) body['vpn_enabled'] = vpn;
      if (tor != null) body['tor_enabled'] = tor;
      final resp = await _httpClient
          .post(
            Uri.parse('$_apiBase/api/paranoidx/config/update'),
            headers: {'Content-Type': 'application/json'},
            body: jsonEncode(body),
          )
          .timeout(const Duration(seconds: 10));
      if (resp.statusCode == 200) {
        return jsonDecode(resp.body) as Map<String, dynamic>;
      }
    } catch (_) {}
    return {'status': 'error'};
  }
}

/// ParanoidXStatus manages data model for ParanoidX multi-layer security status.
class ParanoidXStatus {
  final bool overallHealthy;
  final List<ParanoidXLayer> layers;

  ParanoidXStatus({required this.overallHealthy, required this.layers});

  factory ParanoidXStatus.fromJson(Map<String, dynamic> json) {
    final layers = (json['layers'] as List<dynamic>?)
            ?.map((l) => ParanoidXLayer.fromJson(l as Map<String, dynamic>))
            .toList() ??
        [];
    return ParanoidXStatus(
      overallHealthy: json['overall_healthy'] as bool? ?? false,
      layers: layers,
    );
  }
}

/// ParanoidXLayer manages data model for a single ParanoidX proxy layer status.
class ParanoidXLayer {
  final String layer;
  final bool healthy;
  final int latencyMs;
  final String message;

  ParanoidXLayer({
    required this.layer,
    required this.healthy,
    this.latencyMs = 0,
    this.message = '',
  });

  factory ParanoidXLayer.fromJson(Map<String, dynamic> json) {
    return ParanoidXLayer(
      layer: json['layer'] as String? ?? '',
      healthy: json['healthy'] as bool? ?? false,
      latencyMs: json['latency_ms'] as int? ?? 0,
      message: json['message'] as String? ?? '',
    );
  }
}