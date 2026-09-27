import 'client.dart';

/// LockClient manages lockclient functionality.
class LockClient {
  final SimplexNodeClient _client;
  LockClient(this._client);

  Future<Map<String, dynamic>> status() =>
      _client.get('/api/lock/status');

  Future<Map<String, dynamic>> lock({required String pin}) =>
      _client.post('/api/lock', body: {'pin': pin});

  Future<Map<String, dynamic>> unlock({required String pin}) =>
      _client.post('/api/unlock', body: {'pin': pin});

  Future<Map<String, dynamic>> changeCode({
    required String currentPin,
    required String newPin,
  }) =>
      _client.post('/api/change-code', body: {
        'current_pin': currentPin,
        'new_pin': newPin,
      });
}
