import 'simplex_api_client.dart';
import 'package:models/models.dart';

class TokenClient {
  final SimplexApiClient _client;
  TokenClient(this._client);

  Future<List<Token>> list() async {
    final data = await _client.get('/api/token/list');
    return (data['tokens'] as List).map((e) => Token.fromJson(e)).toList();
  }

  Future<TokenBalancesResponse> balances({required String pubkey}) async {
    final data = await _client.get('/api/token/balances', params: {'pubkey': pubkey});
    return TokenBalancesResponse.fromJson(data);
  }

  Future<TokenOperationResult> addCustom({
    required String symbol,
    required String name,
    int decimals = 18,
    String chain = 'custom',
    String? contractAddress,
    String? logoUrl,
  }) async {
    final data = await _client.post('/api/token/add-custom', body: {
      'symbol': symbol,
      'name': name,
      'decimals': decimals,
      'chain': chain,
      if (contractAddress != null) 'contract_address': contractAddress,
      if (logoUrl != null) 'logo_url': logoUrl,
    });
    return TokenOperationResult.fromJson(data);
  }

  Future<TokenOperationResult> removeCustom(String symbol) async {
    final data = await _client.post('/api/token/remove-custom', body: {'symbol': symbol});
    return TokenOperationResult.fromJson(data);
  }

  Future<TokenOperationResult> updateBalance({
    required String pubkey,
    required String symbol,
    required String balance,
  }) async {
    final data = await _client.post('/api/token/update-balance', body: {
      'pubkey': pubkey,
      'symbol': symbol,
      'balance': balance,
    });
    return TokenOperationResult.fromJson(data);
  }
}

class TokenBalancesResponse {
  final List<TokenBalance> balances;
  TokenBalancesResponse({required this.balances});
  factory TokenBalancesResponse.fromJson(Map<String, dynamic> json) => TokenBalancesResponse(
    balances: (json['balances'] as List).map((e) => TokenBalance.fromJson(e)).toList(),
  );
}

class TokenOperationResult {
  final String status;
  final String? message;
  TokenOperationResult({required this.status, this.message});
  factory TokenOperationResult.fromJson(Map<String, dynamic> json) => TokenOperationResult(
    status: json['status'] ?? 'unknown',
    message: json['message'],
  );
}