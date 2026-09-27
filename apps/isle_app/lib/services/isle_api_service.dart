import 'package:api_client/api_client.dart';
import 'package:http/http.dart' as http;

/// IsleApiService manages convenience wrapper around all simplex-node API client classes.
class IsleApiService {
  final SimplexNodeClient client;
  String pubkey;

  IsleApiService(String baseUrl, this.pubkey, {http.Client? httpClient})
      : client = SimplexNodeClient(baseUrl, client: httpClient);

  WalletClient get wallet => WalletClient(client);
  EconomyClient get economy => EconomyClient(client);
  TreasuryClient get treasury => TreasuryClient(client);
  MarketClient get market => MarketClient(client);
  VaultClient get vault => VaultClient(client);
  AiClient get ai => AiClient(client);
  PosClient get pos => PosClient(client);
  RoyalClient get royal => RoyalClient(client);
  RadioClient get radio => RadioClient(client);
  TokenClient get token => TokenClient(client);
  ExternalWalletClient get externalWallet => ExternalWalletClient(client);

  Future<Map<String, dynamic>> getBalance() =>
      wallet.balance(pubkey: pubkey);

  Future<Map<String, dynamic>> getEconomyState() =>
      economy.state();

  Future<Map<String, dynamic>> getProofOfReserve() =>
      treasury.proofOfReserve();

  Future<Map<String, dynamic>> getHoldings() =>
      economy.holdings(pubkey: pubkey);

  Future<Map<String, dynamic>> transfer(String to, int amount) =>
      wallet.transfer(from: pubkey, to: to, amountNg: amount);

  Future<Map<String, dynamic>> listMarket() =>
      market.list();

  Future<List<dynamic>> getVaultFiles() async {
    final result = await vault.list();
    return result['files'] ?? [];
  }

  Future<Map<String, dynamic>> uploadVaultFile(String path, String name) =>
      vault.upload(path, name);

  Future<List<int>> downloadVaultFile(String name) =>
      vault.download(name);

  Future<Map<String, dynamic>> deleteVaultFile(String name) =>
      vault.delete(name);

  Future<Map<String, dynamic>> getVaultUsage() async {
    final result = await vault.list();
    return {'used_mb': result['used_mb'] ?? 0, 'quota_mb': result['quota_mb'] ?? 2048};
  }

  Future<Map<String, dynamic>> createPosInvoice(int amountNg, {String? description}) =>
      pos.createInvoice(merchant: pubkey, amountNg: amountNg, description: description);

  Future<Map<String, dynamic>> checkPosInvoice(String id) =>
      pos.getInvoice(id);

  Future<Map<String, dynamic>> payPosInvoice(String invoiceId) =>
      pos.payInvoice(invoiceId, pubkey);

  Future<Map<String, dynamic>> listPosInvoices({int limit = 50}) =>
      pos.listInvoices(pubkey, limit: limit);

  String getQrUrl(String invoiceId) =>
      pos.getQrUrl(invoiceId, baseUrl: client.baseUrl);

  void dispose() => client.dispose();
}

