import 'client.dart';

/// PosClient manages posclient functionality.
class PosClient {
  final SimplexNodeClient _client;
  PosClient(this._client);

  Future<Map<String, dynamic>> stats() =>
      _client.get('/api/pos', params: {'action': 'stats'});

  Future<Map<String, dynamic>> createInvoice({
    required String merchant,
    required int amountNg,
    String? description,
  }) =>
      _client.post('/api/pos', params: {'action': 'create-invoice'}, body: {
        'merchant': merchant,
        'amount_ng': amountNg,
        if (description != null) 'description': description,
      });

  Future<Map<String, dynamic>> getInvoice(String id) =>
      _client.get('/api/pos', params: {'action': 'invoice', 'id': id});

  Future<Map<String, dynamic>> listInvoices(String merchant,
          {int limit = 50, int offset = 0}) =>
      _client.get('/api/pos', params: {
        'action': 'list',
        'merchant': merchant,
        'limit': limit.toString(),
        'offset': offset.toString(),
      });

  Future<Map<String, dynamic>> payInvoice(
          String invoiceId, String payer) =>
      _client.post('/api/pos', params: {'action': 'pay'}, body: {
        'invoice_id': invoiceId,
        'payer': payer,
      });

  Future<Map<String, dynamic>> merchantStats(String merchant) =>
      _client.get('/api/pos',
          params: {'action': 'merchant-stats', 'merchant': merchant});

  String getQrUrl(String invoiceId, {String baseUrl = ''}) =>
      '$baseUrl/api/qr?id=$invoiceId';
}
