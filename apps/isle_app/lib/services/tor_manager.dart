import 'dart:io';
import 'package:shared_preferences/shared_preferences.dart';

/// Values for the torstatus domain.
enum TorStatus { unknown, running, stopped, error }

/// TorManager manages tormanager functionality.
class TorManager {
  static const _autoConnectKey = 'tor_auto_connect';
  TorStatus _status = TorStatus.unknown;
  Process? _torProcess;

  TorStatus get status => _status;

/// Returns the current isRunning value.
  bool get isRunning => _status == TorStatus.running;

  Future<bool> get autoConnect async {
    final prefs = await SharedPreferences.getInstance();
    return prefs.getBool(_autoConnectKey) ?? true;
  }

  Future<void> setAutoConnect(bool value) async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setBool(_autoConnectKey, value);
  }

  Future<TorStatus> checkStatus() async {
    try {
      final socket = await Socket.connect('127.0.0.1', 9050, timeout: const Duration(seconds: 2));
      socket.destroy();
      _status = TorStatus.running;
    } on SocketException {
      if (_torProcess != null && _torProcess!.pid > 0) {
        try {
          final r = await Process.run('kill', ['-0', '${_torProcess!.pid}']);
          if (r.exitCode == 0) {
            _status = TorStatus.running;
            return _status;
          }
        } catch (_) {}
      }
      _status = TorStatus.stopped;
    } catch (_) {
      _status = TorStatus.error;
    }
    return _status;
  }

  Future<bool> startTor() async {
    if (isRunning) return true;
    try {
      _torProcess = await Process.start('tor', [
        '--SocksPort', '9050',
        '--DataDirectory', '/tmp/tor-isle',
        '--Log', 'warn stderr',
      ]);
      _status = TorStatus.running;
      _torProcess!.exitCode.then((_) {
        if (_status == TorStatus.running) {
          _status = TorStatus.stopped;
        }
      });
      return true;
    } catch (e) {
      _status = TorStatus.error;
      return false;
    }
  }

  Future<void> stopTor() async {
    if (_torProcess != null) {
      _torProcess!.kill();
      _torProcess = null;
    }
    _status = TorStatus.stopped;
  }

  Future<void> ensureRunning() async {
    if (!isRunning) {
      await checkStatus();
    }
    if (!isRunning) {
      await startTor();
    }
  }

  String get statusLabel {
    switch (_status) {
      case TorStatus.running:
        return 'TOR connected';
      case TorStatus.stopped:
        return 'TOR disconnected';
      case TorStatus.error:
        return 'TOR error';
      case TorStatus.unknown:
        return 'TOR checking...';
    }
  }
}
