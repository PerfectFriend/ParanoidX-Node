import 'client.dart';

/// MarketClient manages marketclient functionality.
class MarketClient {
  final SimplexNodeClient _client;
  MarketClient(this._client);

  Future<Map<String, dynamic>> list() =>
      _client.get('/api/market/list');

  Future<Map<String, dynamic>> sell(String id, int priceNg) =>
      _client.post('/api/market/sell', body: {
        'id': id,
        'price_ng': priceNg,
      });

  Future<Map<String, dynamic>> buy(String id) =>
      _client.post('/api/market/buy', body: {'id': id});

  Future<Map<String, dynamic>> escrowCreate(String buyer, String seller, String itemId, int priceNg) =>
      _client.post('/api/escrow/create', body: {
        'buyer': buyer,
        'seller': seller,
        'item_id': itemId,
        'price_ng': priceNg,
      });

  Future<Map<String, dynamic>> escrowRelease(String escrowId) =>
      _client.get('/api/escrow/release', params: {'id': escrowId});

  Future<Map<String, dynamic>> escrowList({String? status}) =>
      _client.get('/api/escrow/list', params: {
        if (status != null) 'status': status,
      });
}
