import 'client.dart';

/// BillingClient manages billingclient functionality.
class BillingClient {
  final SimplexNodeClient _client;
  BillingClient(this._client);

  Future<Map<String, dynamic>> getPrices() =>
      _client.get('/api/billing/prices');

  Future<Map<String, dynamic>> pay({
    required String service,
    required int amountNg,
    String? from,
  }) =>
      _client.post('/api/billing/pay', body: {
        'service': service,
        'amount_ng': amountNg,
        if (from != null) 'from': from,
      });

  Future<Map<String, dynamic>> history({int? limit}) =>
      _client.get('/api/billing/history',
          params: limit != null ? {'limit': limit.toString()} : null);
}
