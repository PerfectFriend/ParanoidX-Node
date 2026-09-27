import 'dart:convert';
import 'package:http/http.dart' as http;

/// SimpleXTransportConfig manages simplextransportconfig functionality.
class SimpleXTransportConfig {
  final String smp;
  final String xftp;
  final String ice;
  final String onion;
  final String contact;
  final String label;

  SimpleXTransportConfig({
    required this.smp,
    required this.xftp,
    required this.ice,
    required this.onion,
    required this.contact,
    required this.label,
  });

/// Returns the current hasSMP value.
  bool get hasSMP => smp.isNotEmpty;

/// Returns the current hasOnion value.
  bool get hasOnion => onion.isNotEmpty;

  factory SimpleXTransportConfig.fromJson(Map<String, dynamic> json) {
    return SimpleXTransportConfig(
      smp: json['smp'] as String? ?? '',
      xftp: json['xftp'] as String? ?? '',
      ice: json['ice'] as String? ?? '',
      onion: json['onion'] as String? ?? '',
      contact: json['contact'] as String? ?? '',
      label: json['label'] as String? ?? '',
    );
  }

  factory SimpleXTransportConfig.empty() {
    return SimpleXTransportConfig(smp: '', xftp: '', ice: '', onion: '', contact: '', label: '');
  }
}

/// SimpleXTransportLoader manages simplextransportloader functionality.
class SimpleXTransportLoader {
  static Future<SimpleXTransportConfig> load(String serverUrl, {http.Client? client}) async {
    final c = client ?? http.Client();
    try {
      final uri = Uri.parse('$serverUrl/api/transport/info');
      final resp = await c.get(uri).timeout(const Duration(seconds: 5));
      if (resp.statusCode == 200) {
        final json = jsonDecode(resp.body) as Map<String, dynamic>;
        return SimpleXTransportConfig.fromJson(json);
      }
    } catch (_) {
      // node not reachable
    } finally {
      if (client == null) c.close();
    }
    return SimpleXTransportConfig.empty();
  }

  static Future<bool> testSMP(String smpAddress) async {
    // Basic validation — check the address format
    return smpAddress.startsWith('smp://') && smpAddress.contains('@') && smpAddress.contains('.onion');
  }
}
