import 'client.dart';

/// EconomyClient manages economyclient functionality.
class EconomyClient {
  final SimplexNodeClient _client;
  EconomyClient(this._client);

  Future<Map<String, dynamic>> state() =>
      _client.get('/api/economy/state');

  Future<Map<String, dynamic>> holdings({required String pubkey}) =>
      _client.get('/api/economy/holdings', params: {'pubkey': pubkey});

  Future<Map<String, dynamic>> history({required String pubkey}) =>
      _client.get('/api/economy/history', params: {'pubkey': pubkey});
}
