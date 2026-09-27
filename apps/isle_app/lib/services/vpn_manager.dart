import 'dart:async';
import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:http/http.dart' as http;

/// Values for the vpnstatus domain.
enum VpnStatus { disconnected, configuring, connected, error }

/// VpnInfo manages vpninfo functionality.
class VpnInfo {
  final VpnStatus status;
  final String version;
  final String error;

  const VpnInfo({this.status = VpnStatus.disconnected, this.version = '', this.error = ''});
}

/// VpnManager manages monitors the WireGuard VPN connection status.
class VpnManager extends ValueNotifier<VpnInfo> {
  final http.Client _httpClient;
  Timer? _checkTimer;

  VpnManager({required http.Client httpClient})
      : _httpClient = httpClient,
        super(const VpnInfo());

  void start() {
    _checkTimer = Timer.periodic(const Duration(seconds: 30), (_) => check());
    check();
  }

  void stop() {
    _checkTimer?.cancel();
  }

  Future<void> check() async {
    try {
      final resp = await _httpClient
          .get(Uri.parse('http://127.0.0.1:2018/api/version'))
          .timeout(const Duration(seconds: 5));
      if (resp.statusCode == 200) {
        final data = jsonDecode(resp.body) as Map<String, dynamic>;
        if (data['code'] == 'SUCCESS') {
          final ver = data['data']['version'] as String? ?? '';
          final valid = data['data']['serviceValid'] == true;
          value = VpnInfo(
            status: valid ? VpnStatus.connected : VpnStatus.configuring,
            version: ver,
          );
          return;
        }
      }
      value = const VpnInfo(status: VpnStatus.disconnected);
    } catch (_) {
      value = const VpnInfo(status: VpnStatus.disconnected);
    }
  }

  @override
  void dispose() {
    _checkTimer?.cancel();
    super.dispose();
  }
}
