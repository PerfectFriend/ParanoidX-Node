import 'client.dart';

/// StatusClient manages statusclient functionality.
class StatusClient {
  final SimplexNodeClient _client;
  StatusClient(this._client);

  Future<Map<String, dynamic>> getStatus() =>
      _client.get('/api/status');

  Future<Map<String, dynamic>> health() =>
      _client.get('/api/health');

  Future<Map<String, dynamic>> reputation({String? pubkey}) =>
      _client.get('/api/reputation', params: pubkey != null ? {'pubkey': pubkey} : null);
}
