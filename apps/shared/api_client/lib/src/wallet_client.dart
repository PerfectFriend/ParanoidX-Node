import 'client.dart';

/// WalletClient manages walletclient functionality.
class WalletClient {
  final SimplexNodeClient _client;
  WalletClient(this._client);

  Future<Map<String, dynamic>> create() =>
      _client.get('/api/wallet/create');

  Future<Map<String, dynamic>> balance({required String pubkey}) =>
      _client.get('/api/wallet/balance', params: {'pubkey': pubkey});

  Future<Map<String, dynamic>> transfer({
    required String from,
    required String to,
    required int amountNg,
  }) =>
      _client.post('/api/wallet/send', body: {
        'from': from,
        'to': to,
        'amount_ng': amountNg,
      });
}
