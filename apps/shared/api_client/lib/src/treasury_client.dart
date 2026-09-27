import 'client.dart';

/// TreasuryClient manages treasuryclient functionality.
class TreasuryClient {
  final SimplexNodeClient _client;
  TreasuryClient(this._client);

  Future<Map<String, dynamic>> proofOfReserve() =>
      _client.get('/api/treasury/proof-of-reserve');

  Future<Map<String, dynamic>> state() =>
      _client.get('/api/treasury/state');

  Future<Map<String, dynamic>> claimDividends({
    required String serial,
    required String holder,
  }) =>
      _client.post('/api/treasury/claim-dividends', body: {
        'serial': serial,
        'holder': holder,
      });

  Future<Map<String, dynamic>> autoRound({double threshold = 100}) =>
      _client.get('/api/treasury/auto-round', params: {
        'threshold': threshold.toString(),
      });
}
