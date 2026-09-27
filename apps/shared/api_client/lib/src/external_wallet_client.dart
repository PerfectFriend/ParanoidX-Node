import 'simplex_api_client.dart';
import 'package:models/models.dart';
import 'package:models/models.dart' show ExternalWallet;

class ExternalWalletClient {
  final SimplexApiClient _client;
  ExternalWalletClient(this._client);

  Future<List<ExternalWallet>> list({required String pubkey}) async {
    final data = await _client.get('/api/external-wallet/list', params: {'pubkey': pubkey});
    return (data['wallets'] as List).map((e) => ExternalWallet.fromJson(e)).toList();
  }

  Future<ExternalWalletOperationResult> link({
    required String pubkey,
    required String walletType,
    required String walletAddress,
    String? label,
    String? chain,
  }) async {
    final data = await _client.post('/api/external-wallet/link', body: {
      'pubkey': pubkey,
      'wallet_type': walletType,
      'wallet_address': walletAddress,
      if (label != null) 'label': label,
      if (chain != null) 'chain': chain,
    });
    return ExternalWalletOperationResult.fromJson(data);
  }

  Future<ExternalWalletOperationResult> unlink(String pubkey, String walletType) async {
    final data = await _client.post('/api/external-wallet/unlink', body: {
      'pubkey': pubkey,
      'wallet_type': walletType,
    });
    return ExternalWalletOperationResult.fromJson(data);
  }

  Future<ExternalWalletOperationResult> sync(String pubkey) async {
    final data = await _client.post('/api/external-wallet/sync', body: {'pubkey': pubkey});
    return ExternalWalletOperationResult.fromJson(data);
  }

  Future<ExternalWalletOperationResult> verify(String pubkey, String walletType) async {
    final data = await _client.post('/api/external-wallet/verify', body: {
      'pubkey': pubkey,
      'wallet_type': walletType,
    });
    return ExternalWalletOperationResult.fromJson(data);
  }
}

class ExternalWalletOperationResult {
  final String status;
  final String? message;
  final ExternalWallet? wallet;
  ExternalWalletOperationResult({required this.status, this.message, this.wallet});
  factory ExternalWalletOperationResult.fromJson(Map<String, dynamic> json) => ExternalWalletOperationResult(
    status: json['status'] ?? 'unknown',
    message: json['message'],
    wallet: json['wallet'] != null ? ExternalWallet.fromJson(json['wallet']) : null,
  );
}