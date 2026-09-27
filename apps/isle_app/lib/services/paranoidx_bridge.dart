import 'dart:async';
import 'package:flutter/services.dart';

/// ParanoidXBridge manages platform channel bridge for ParanoidX native functionality.
class ParanoidXBridge {
  static const _channel = MethodChannel('com.example.isle_app/paranoidx');

  final StreamController<Map<String, dynamic>> _statusCtrl =
      StreamController<Map<String, dynamic>>.broadcast();

  Stream<Map<String, dynamic>> get statusStream => _statusCtrl.stream;

  ParanoidXBridge() {
    _channel.setMethodCallHandler(_handleMethodCall);
  }

  Future<Map<String, dynamic>> getStatus() async {
    try {
      final result = await _channel.invokeMethod<Map>('getStatus');
      return Map<String, dynamic>.from(result ?? {});
    } catch (e) {
      return {'error': e.toString()};
    }
  }

  Future<void> start() async {
    await _channel.invokeMethod('start');
  }

  Future<void> stop() async {
    await _channel.invokeMethod('stop');
  }

  Future<Map<String, dynamic>> checkChain() async {
    try {
      final result = await _channel.invokeMethod<Map>('checkChain');
      return Map<String, dynamic>.from(result ?? {});
    } catch (e) {
      return {'error': e.toString()};
    }
  }

  Future<void> _handleMethodCall(MethodCall call) async {
    switch (call.method) {
      case 'status':
        _statusCtrl.add(Map<String, dynamic>.from(call.arguments as Map));
        break;
    }
  }

  void dispose() {
    _statusCtrl.close();
  }
}
