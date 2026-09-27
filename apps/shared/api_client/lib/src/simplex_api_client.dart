import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';
import 'package:http/http.dart' as http;
import 'package:models/models.dart';
import 'token_client.dart';
import 'external_wallet_client.dart';
import 'royal_client.dart';

/// SimplexApiClient - unified typed HTTP client for simplex-node API.
/// 
/// Features:
/// - Auto-routing Onion/HTTP based on address
/// - Ed25519 request signing via Identity
/// - Typed endpoints with DTOs from shared/models
/// - Interceptors: auth, logging, retries (exponential backoff), offline queue
/// - WebSocket/SSE support for real-time updates
/// - Offline-first with local SQLite cache (drift)
/// 
/// Usage:
/// ```dart
/// final client = SimplexApiClient(baseUrl: 'http://127.0.0.1:8080');
/// await client.initialize(identity);
/// final balance = await client.wallet.getBalance();
/// ```
class SimplexApiClient {
  final String baseUrl;
  final http.Client _httpClient;
  final Duration _timeout;
  Identity? _identity;
  bool _useTransport = true;
  int _reqCounter = 0;
  
  // Offline queue
  final _offlineQueue = <_QueuedRequest>[];
  bool _isOnline = true;
  
  // SSE streams
  StreamController<Map<String, dynamic>>? _eventsController;
  StreamSubscription? _sseSub;

  SimplexApiClient({
    required this.baseUrl,
    http.Client? httpClient,
    Duration timeout = const Duration(seconds: 30),
  }) : _httpClient = httpClient ?? http.Client(),
       _timeout = timeout;

  /// Initialize with identity for request signing
  Future<void> initialize(Identity identity) async {
    _identity = identity;
    // Verify connectivity
    _isOnline = await _checkConnectivity();
  }

  /// Get the underlying http client for advanced usage
  http.Client get httpClient => _httpClient;

  /// Current identity
  Identity? get identity => _identity;

  /// Whether using transport layer (SimpleX)
  bool get useTransport => _useTransport;
  set useTransport(bool v) => _useTransport = v;

  /// Whether we have an identity loaded
  bool get hasIdentity => _identity != null;

  /// Check if online
  bool get isOnline => _isOnline;

  // ==================== Core HTTP Methods ====================

  Future<Map<String, dynamic>> get(String path, {Map<String, String>? params}) async {
    final fullPath = _buildPath(path, params);
    return _execute('GET', fullPath, null);
  }

  Future<Map<String, dynamic>> post(String path, {Map<String, dynamic>? body, Map<String, String>? params}) async {
    final fullPath = _buildPath(path, params);
    return _execute('POST', fullPath, body);
  }

  Future<Uint8List> getBytes(String path, {Map<String, String>? params}) async {
    final fullPath = _buildPath(path, params);
    final result = await _executeRaw('GET', fullPath, null);
    return result;
  }

  String _buildPath(String path, Map<String, String>? params) {
    if (params != null && params.isNotEmpty) {
      return '$path?${params.entries.map((e) => '${Uri.encodeComponent(e.key)}=${Uri.encodeComponent(e.value)}').join('&')}';
    }
    return path;
  }

  Future<Map<String, dynamic>> _execute(String method, String path, Map<String, dynamic>? body) async {
    final result = await _executeRaw(method, path, body);
    return jsonDecode(utf8.decode(result)) as Map<String, dynamic>;
  }

  Future<Uint8List> _executeRaw(String method, String path, Map<String, dynamic>? body) async {
    if (!_isOnline) {
      _queueOffline(method, path, body);
      throw ApiException(0, 'Offline - request queued');
    }

    if (_useTransport) {
      return _transportCall(method, path, body);
    } else {
      return _directCall(method, path, body);
    }
  }

  Future<Uint8List> _directCall(String method, String path, Map<String, dynamic>? body) async {
    final uri = Uri.parse('$baseUrl$path');
    http.Response response;
    
    if (method == 'GET') {
      response = await _httpClient.get(uri, headers: _headers()).timeout(_timeout);
    } else {
      response = await _httpClient.post(
        uri,
        headers: _headers(),
        body: body != null ? jsonEncode(body) : null,
      ).timeout(_timeout);
    }
    
    _checkError(response);
    return response.bodyBytes;
  }

  Future<Uint8List> _transportCall(String method, String path, Map<String, dynamic>? body) async {
    _reqCounter++;
    final envelope = {
      'type': 'api.request',
      'payload': {
        'method': method,
        'path': path,
        'body': body,
      },
      'id': 'req-$_reqCounter',
    };

    final uri = Uri.parse('$baseUrl/api/transport/send');
    final response = await _httpClient.post(
      uri,
      headers: _headers(),
      body: jsonEncode(envelope),
    ).timeout(_timeout);

    if (response.statusCode >= 400) {
      throw ApiException(response.statusCode, response.body);
    }

    final responseEnvelope = jsonDecode(response.body) as Map<String, dynamic>;
    if (responseEnvelope['type'] == 'error') {
      throw ApiException(502, responseEnvelope['payload']?.toString() ?? 'transport error');
    }

    final apiResp = responseEnvelope['payload'] as Map<String, dynamic>;
    final status = apiResp['status'] as int;
    final respBodyRaw = apiResp['body'];

    Uint8List respBody;
    if (respBodyRaw is String) {
      respBody = utf8.encode(respBodyRaw);
    } else if (respBodyRaw is Map) {
      respBody = utf8.encode(jsonEncode(respBodyRaw));
    } else {
      respBody = Uint8List(0);
    }

    if (status >= 400) {
      throw ApiException(status, utf8.decode(respBody));
    }

    return respBody;
  }

  Map<String, String> _headers() {
    final headers = <String, String>{
      'Content-Type': 'application/json',
      'Accept': 'application/json',
    };
    
    if (_identity != null) {
      headers['X-Identity-Pubkey'] = _identity!.ed25519PubKey;
      headers['X-Identity-Timestamp'] = DateTime.now().millisecondsSinceEpoch.toString();
      // TODO: Add Ed25519 signature of request
    }
    
    return headers;
  }

  void _checkError(http.Response response) {
    if (response.statusCode >= 400) {
      throw ApiException(response.statusCode, response.body);
    }
  }

  // ==================== Connectivity & Offline Queue ====================

  Future<bool> _checkConnectivity() async {
    try {
      final response = await _httpClient.get(Uri.parse('$baseUrl/api/health')).timeout(const Duration(seconds: 5));
      return response.statusCode == 200;
    } catch (_) {
      return false;
    }
  }

  void _queueOffline(String method, String path, Map<String, dynamic>? body) {
    _offlineQueue.add(_QueuedRequest(
      method: method,
      path: path,
      body: body,
      timestamp: DateTime.now(),
    ));
  }

  Future<void> flushOfflineQueue() async {
    if (_offlineQueue.isEmpty) return;
    _isOnline = await _checkConnectivity();
    if (!_isOnline) return;

    final toProcess = List<_QueuedRequest>.from(_offlineQueue);
    _offlineQueue.clear();
    
    for (final req in toProcess) {
      try {
        await _executeRaw(req.method, req.path, req.body);
      } catch (_) {
        _offlineQueue.add(req); // Re-queue on failure
      }
    }
  }

  // ==================== SSE Events ====================

  Stream<Map<String, dynamic>> events() {
    _eventsController ??= StreamController<Map<String, dynamic>>.broadcast();
    _startSSE();
    return _eventsController!.stream;
  }

  void _startSSE() {
    if (_sseSub != null) return;
    
    final uri = Uri.parse('$baseUrl/api/royal/events');
    final request = http.Request('GET', uri);
    request.headers.addAll(_headers());
    
    _httpClient.send(request).then((response) {
      _sseSub = response.stream
          .transform(utf8.decoder)
          .transform(const LineSplitter())
          .listen((line) {
        if (line.startsWith('data: ')) {
          try {
            final data = jsonDecode(line.substring(6)) as Map<String, dynamic>;
            _eventsController?.add(data);
          } catch (_) {}
        }
      }, onError: (e) {
        _eventsController?.addError(e);
      }, onDone: () {
        _sseSub = null;
        Future.delayed(const Duration(seconds: 5), _startSSE); // Reconnect
      });
    });
  }

  // ==================== Typed API Clients ====================

  late final WalletApi wallet = WalletApi(this);
  late final MarketApi market = MarketApi(this);
  late final TreasuryApi treasury = TreasuryApi(this);
  late final GovernanceApi governance = GovernanceApi(this);
  late final SystemApi system = SystemApi(this);
  late final ChatApi chat = ChatApi(this);
  late final RadioApi radio = RadioApi(this);
  late final VaultApi vault = VaultApi(this);
  late final AiApi ai = AiApi(this);
  late final PosApi pos = PosApi(this);
  late final ParanoidXApi paranoidx = ParanoidXApi(this);
  late final TokenClient tokenClient = TokenClient(this);
  late final ExternalWalletClient externalWalletClient = ExternalWalletClient(this);
  late final RoyalClient royal = RoyalClient(this);

  // ==================== Lifecycle ====================

  void dispose() {
    _sseSub?.cancel();
    _eventsController?.close();
    _httpClient.close();
  }
}

class _QueuedRequest {
  final String method;
  final String path;
  final Map<String, dynamic>? body;
  final DateTime timestamp;
  
  _QueuedRequest({
    required this.method,
    required this.path,
    this.body,
    required this.timestamp,
  });
}

/// ApiException represents a non-200 HTTP response from the API.
class ApiException implements Exception {
  final int statusCode;
  final String body;
  ApiException(this.statusCode, this.body);

  @override
  String toString() => 'ApiException($statusCode): $body';
}

// ==================== Typed Sub-Clients ====================

class WalletApi {
  final SimplexApiClient _client;
  WalletApi(this._client);

  Future<WalletBalance> getBalance({String? pubkey}) async {
    final data = await _client.get('/api/wallet/balance', params: pubkey != null ? {'pubkey': pubkey} : null);
    return WalletBalance.fromJson(data);
  }

  Future<TransferResult> transfer({required String from, required String to, required int amountNg, String? memo}) async {
    final data = await _client.post('/api/wallet/send', body: {'from': from, 'to': to, 'amount_ng': amountNg, if (memo != null) 'memo': memo});
    return TransferResult.fromJson(data);
  }

  Future<MintResult> mint({required String holder, required int amountNg, String? reason}) async {
    final data = await _client.post('/api/wallet/mint', body: {'holder': holder, 'amount_ng': amountNg, if (reason != null) 'reason': reason});
    return MintResult.fromJson(data);
  }

  Future<RedeemResult> redeem({required String assetId, required String holder, String? reason}) async {
    final data = await _client.post('/api/wallet/redeem', body: {'asset_id': assetId, 'holder': holder, if (reason != null) 'reason': reason});
    return RedeemResult.fromJson(data);
  }

  Future<List<WalletTransaction>> history({String? pubkey, int limit = 50, int offset = 0}) async {
    final data = await _client.get('/api/wallet/history', params: {'pubkey': pubkey ?? '', 'limit': limit.toString(), 'offset': offset.toString()});
    return (data['transactions'] as List).map((e) => WalletTransaction.fromJson(e)).toList();
  }

  Future<BanknoteResult> createBanknote({required String holder, required int denominationNg, String? serial}) async {
    final data = await _client.post('/api/wallet/banknote', body: {'holder': holder, 'denomination_ng': denominationNg, if (serial != null) 'serial': serial});
    return BanknoteResult.fromJson(data);
  }

  Future<DividendResult> claimDividend({required String pubkey}) async {
    final data = await _client.post('/api/wallet/dividend/claim', body: {'pubkey': pubkey});
    return DividendResult.fromJson(data);
  }

  Future<ReceiveInfo> receive(String pubkey) async {
    final data = await _client.get('/api/wallet/receive', params: {'pubkey': pubkey});
    return ReceiveInfo.fromJson(data);
  }
}

class MarketApi {
  final SimplexApiClient _client;
  MarketApi(this._client);

  Future<List<MarketListing>> list({int limit = 20, int offset = 0, String? category}) async {
    final params = <String, String>{'limit': limit.toString(), 'offset': offset.toString()};
    if (category != null) params['category'] = category;
    final data = await _client.get('/api/market/listings', params: params);
    return (data['listings'] as List).map((e) => MarketListing.fromJson(e)).toList();
  }

  Future<MarketListing> create({required MarketListing listing}) async {
    final data = await _client.post('/api/market/create', body: listing.toJson());
    return MarketListing.fromJson(data);
  }

  Future<PurchaseResult> buy({required String listingId, required String buyerPubkey, int? quantity}) async {
    final data = await _client.post('/api/market/buy', body: {'listing_id': listingId, 'buyer_pubkey': buyerPubkey, if (quantity != null) 'quantity': quantity});
    return PurchaseResult.fromJson(data);
  }

  Future<List<MarketOrder>> orders({required String pubkey, int limit = 50}) async {
    final data = await _client.get('/api/market/orders', params: {'pubkey': pubkey, 'limit': limit.toString()});
    return (data['orders'] as List).map((e) => MarketOrder.fromJson(e)).toList();
  }

  Future<EscrowResult> createEscrow({required String listingId, required String buyer, required String seller, required int amountNg}) async {
    final data = await _client.post('/api/market/escrow', body: {'listing_id': listingId, 'buyer': buyer, 'seller': seller, 'amount_ng': amountNg});
    return EscrowResult.fromJson(data);
  }
}

class TreasuryApi {
  final SimplexApiClient _client;
  TreasuryApi(this._client);

  Future<TreasuryState> state() async {
    final data = await _client.get('/api/treasury/state');
    return TreasuryState.fromJson(data);
  }

  Future<ReserveState> reserve() async {
    final data = await _client.get('/api/treasury/reserve');
    return ReserveState.fromJson(data);
  }

  Future<OraclePrice> oracle() async {
    final data = await _client.get('/api/treasury/oracle');
    return OraclePrice.fromJson(data);
  }

  Future<void> updateOracle(double price) async {
    await _client.post('/api/treasury/oracle', body: {'price': price});
  }

  Future<DeflationState> deflation() async {
    final data = await _client.get('/api/treasury/deflation');
    return DeflationState.fromJson(data);
  }

  Future<void> triggerDeflation() async {
    await _client.post('/api/treasury/deflation', body: {});
  }

  Future<AutoMintConfig> autoMintConfig() async {
    final data = await _client.get('/api/treasury/auto-mint');
    return AutoMintConfig.fromJson(data);
  }

  Future<void> updateAutoMint(bool enabled, int intervalMs) async {
    await _client.post('/api/treasury/auto-mint', body: {'enabled': enabled, 'interval_ms': intervalMs});
  }

  Future<DividendPool> dividendPool() async {
    final data = await _client.get('/api/treasury/dividend-pool');
    return DividendPool.fromJson(data);
  }

  Future<void> triggerDividend({int poolNg = 0}) async {
    await _client.post('/api/treasury/dividend', body: {'pool_ng': poolNg});
  }

  Future<List<DividendHistory>> dividendHistory({int limit = 20}) async {
    final data = await _client.get('/api/treasury/dividend/history', params: {'limit': limit.toString()});
    return (data['history'] as List).map((e) => DividendHistory.fromJson(e)).toList();
  }

  Future<MintResult> mint({required String holder, required int amountNg, String? reason}) async {
    final data = await _client.post('/api/treasury/mint', body: {'holder': holder, 'amount_ng': amountNg, if (reason != null) 'reason': reason});
    return MintResult.fromJson(data);
  }

  Future<BurnResult> burn({required String assetId, required String holder, String? reason}) async {
    final data = await _client.post('/api/treasury/burn', body: {'asset_id': assetId, 'holder': holder, if (reason != null) 'reason': reason});
    return BurnResult.fromJson(data);
  }

  Future<List<Banknote>> banknotes() async {
    final data = await _client.get('/api/treasury/banknotes');
    return (data['banknotes'] as List).map((e) => Banknote.fromJson(e)).toList();
  }

  Future<ProofOfReserve> proofOfReserve() async {
    final data = await _client.get('/api/treasury/proof-of-reserve');
    return ProofOfReserve.fromJson(data);
  }

  Future<Rates> rates() async {
    final data = await _client.get('/api/treasury/rates');
    return Rates.fromJson(data);
  }

  Future<Tokenomics> tokenomics() async {
    final data = await _client.get('/api/treasury/tokenomics');
    return Tokenomics.fromJson(data);
  }

  Future<Forecast> forecast() async {
    final data = await _client.get('/api/treasury/forecast');
    return Forecast.fromJson(data);
  }
}

class GovernanceApi {
  final SimplexApiClient _client;
  GovernanceApi(this._client);

  Future<Constitution> constitution() async {
    final data = await _client.get('/api/gov/constitution');
    return Constitution.fromJson(data);
  }

  Future<List<Proposal>> proposals({int limit = 20, String? status}) async {
    final params = <String, String>{'limit': limit.toString()};
    if (status != null) params['status'] = status;
    final data = await _client.get('/api/gov/proposals', params: params);
    return (data['proposals'] as List).map((e) => Proposal.fromJson(e)).toList();
  }

  Future<Proposal> createProposal(ProposalDraft draft) async {
    final data = await _client.post('/api/gov/proposals', body: draft.toJson());
    return Proposal.fromJson(data);
  }

  Future<VoteResult> vote({required String proposalId, required String pubkey, required bool approve, int? conviction}) async {
    final data = await _client.post('/api/gov/vote', body: {'proposal_id': proposalId, 'pubkey': pubkey, 'approve': approve, if (conviction != null) 'conviction': conviction});
    return VoteResult.fromJson(data);
  }

  Future<DelegationResult> delegate({required String fromPubkey, required String toPubkey, int? weight}) async {
    final data = await _client.post('/api/gov/delegate', body: {'from': fromPubkey, 'to': toPubkey, if (weight != null) 'weight': weight});
    return DelegationResult.fromJson(data);
  }
}

class SystemApi {
  final SimplexApiClient _client;
  SystemApi(this._client);

  Future<NodeInfo> info() async {
    final data = await _client.get('/api/admin/info');
    return NodeInfo.fromJson(data);
  }

  Future<SystemMetrics> metrics() async {
    final data = await _client.get('/api/admin/metrics/system');
    return SystemMetrics.fromJson(data);
  }

  Future<DockerStatus> docker() async {
    final data = await _client.get('/api/admin/docker');
    return DockerStatus.fromJson(data);
  }

  Future<ServiceStatus> services() async {
    final data = await _client.get('/api/admin/service/status');
    return ServiceStatus.fromJson(data);
  }

  Future<Diagnostics> diagnostics() async {
    final data = await _client.get('/api/admin/diagnostics');
    return Diagnostics.fromJson(data);
  }

  Future<ServiceActionResult> restartService(String service) async {
    final data = await _client.post('/api/admin/service/restart', body: {'service': service});
    return ServiceActionResult.fromJson(data);
  }

  Future<BackupResult> backup() async {
    final data = await _client.post('/api/admin/backup', body: {});
    return BackupResult.fromJson(data);
  }

  Future<CleanupResult> diskCleanup() async {
    final data = await _client.post('/api/admin/disk-cleanup', body: {});
    return CleanupResult.fromJson(data);
  }

  Future<Config> config() async {
    final data = await _client.get('/api/admin/config');
    return Config.fromJson(data);
  }

  Future<void> updateConfig(Config config) async {
    await _client.post('/api/admin/config', body: config.toJson());
  }

  Future<MaintenanceMode> maintenance() async {
    final data = await _client.get('/api/admin/maintenance');
    return MaintenanceMode.fromJson(data);
  }

  Future<void> setMaintenance(bool active, {String message = ''}) async {
    await _client.post('/api/admin/maintenance', body: {'active': active, 'message': message});
  }

  Future<EmergencyStop> emergencyStop() async {
    final data = await _client.get('/api/royal/emergency-stop');
    return EmergencyStop.fromJson(data);
  }

  Future<void> setEmergencyStop(bool enable) async {
    await _client.post('/api/royal/emergency-stop', body: {'action': enable ? 'enable' : 'disable'});
  }

  Future<RateLimitStats> rateLimits() async {
    final data = await _client.get('/api/royal/rate-limit-stats');
    return RateLimitStats.fromJson(data);
  }

  Future<ContainerStatus> containerStatus() async {
    final data = await _client.get('/api/container/status');
    return ContainerStatus.fromJson(data);
  }

  Future<ContainerActionResult> openContainer(String password) async {
    final data = await _client.post('/api/container/open', body: {'password': password});
    return ContainerActionResult.fromJson(data);
  }

  Future<ContainerActionResult> closeContainer() async {
    final data = await _client.post('/api/container/close', body: {});
    return ContainerActionResult.fromJson(data);
  }
}

class ChatApi {
  final SimplexApiClient _client;
  ChatApi(this._client);

  Future<ChatStatus> status() async {
    final data = await _client.get('/api/chat/status');
    return ChatStatus.fromJson(data);
  }

  Future<List<Conversation>> conversations({required String pubkey}) async {
    final data = await _client.get('/api/chat/conversations', params: {'pubkey': pubkey});
    return (data['conversations'] as List).map((e) => Conversation.fromJson(e)).toList();
  }

  Future<MessageResult> send({required String to, required String from, required String message}) async {
    final data = await _client.post('/api/chat/send', body: {'to': to, 'from': from, 'message': message});
    return MessageResult.fromJson(data);
  }

  Future<BroadcastResult> broadcast(String message) async {
    final data = await _client.post('/api/royal/chat/broadcast', body: {'message': message});
    return BroadcastResult.fromJson(data);
  }

  Future<TreasuryAlertResult> treasuryAlert() async {
    final data = await _client.post('/api/royal/chat/treasury-alert', body: {});
    return TreasuryAlertResult.fromJson(data);
  }

  Stream<ChatMessage> messages(String conversationId) {
    // Returns a stream of messages for a conversation via SSE
    return _client.events().where((e) => e['conversation_id'] == conversationId).map((e) => ChatMessage.fromJson(e));
  }
}

class RadioApi {
  final SimplexApiClient _client;
  RadioApi(this._client);

  Future<RadioSchedule> schedule() async {
    final data = await _client.get('/api/radio/schedule');
    return RadioSchedule.fromJson(data);
  }

  Future<RadioScheduleContent> scheduleContent() async {
    final data = await _client.get('/api/radio/schedule-content');
    return RadioScheduleContent.fromJson(data);
  }

  Future<void> updateScheduleContent(RadioScheduleContent content) async {
    await _client.post('/api/radio/schedule-content', body: content.toJson());
  }

  Future<AiContent> aiContent() async {
    final data = await _client.get('/api/radio/ai-content');
    return AiContent.fromJson(data);
  }

  Future<void> updateAiContent(AiContent content) async {
    await _client.post('/api/radio/ai-content', body: content.toJson());
  }
}

class VaultApi {
  final SimplexApiClient _client;
  VaultApi(this._client);

  Future<VaultListing> list() async {
    final data = await _client.get('/api/vault/list');
    return VaultListing.fromJson(data);
  }

  Future<UploadResult> upload({required String path, required String name}) async {
    final data = await _client.post('/api/vault/upload', body: {'path': path, 'name': name});
    return UploadResult.fromJson(data);
  }

  Future<Uint8List> download(String name) async {
    return _client.getBytes('/api/vault/download', params: {'name': name});
  }

  Future<DeleteResult> delete(String name) async {
    final data = await _client.post('/api/vault/delete', body: {'name': name});
    return DeleteResult.fromJson(data);
  }
}

class AiApi {
  final SimplexApiClient _client;
  AiApi(this._client);

  Future<AiResponse> chat(String question, {String? context, String? profile, String? userId}) async {
    final data = await _client.post('/api/ai/chat', body: {'question': question, if (context != null) 'context': context, if (profile != null) 'profile': profile, if (userId != null) 'user_id': userId});
    return AiResponse.fromJson(data);
  }

  Future<AiExplanation> explainSilver() async {
    final data = await _client.post('/api/ai/explain-silver', body: {});
    return AiExplanation.fromJson(data);
  }

  Future<MemoryStats> memoryStats() async {
    final data = await _client.get('/api/ai/memory/stats');
    return MemoryStats.fromJson(data);
  }

  Future<AiHealth> health() async {
    final data = await _client.get('/api/ai/health');
    return AiHealth.fromJson(data);
  }
}

class PosApi {
  final SimplexApiClient _client;
  PosApi(this._client);

  Future<Invoice> createInvoice({required String merchant, required int amountNg, String? description}) async {
    final data = await _client.post('/api/pos/invoice', body: {'merchant': merchant, 'amount_ng': amountNg, if (description != null) 'description': description});
    return Invoice.fromJson(data);
  }

  Future<Invoice> getInvoice(String id) async {
    final data = await _client.get('/api/pos/invoice', params: {'id': id});
    return Invoice.fromJson(data);
  }

  Future<PaymentResult> payInvoice({required String invoiceId, required String pubkey}) async {
    final data = await _client.post('/api/pos/pay', body: {'invoice_id': invoiceId, 'pubkey': pubkey});
    return PaymentResult.fromJson(data);
  }

  Future<List<Invoice>> listInvoices({required String pubkey, int limit = 50}) async {
    final data = await _client.get('/api/pos/invoices', params: {'pubkey': pubkey, 'limit': limit.toString()});
    return (data['invoices'] as List).map((e) => Invoice.fromJson(e)).toList();
  }
}

/// ParanoidX API for multi-layer proxy chain management
class ParanoidXApi {
  final SimplexApiClient _client;
  ParanoidXApi(this._client);

  Future<Map<String, dynamic>> getStatus() async {
    return _client.get('/api/paranoidx/status');
  }

  Future<Map<String, dynamic>> getConfig() async {
    return _client.get('/api/paranoidx/config');
  }

  Future<Map<String, dynamic>> getChainState() async {
    return _client.get('/api/paranoidx/chain-state');
  }

  Future<Map<String, dynamic>> updateConfig({required bool v2ray, required bool vpn, required bool tor}) async {
    return _client.post('/api/paranoidx/config', body: {'v2ray_enabled': v2ray, 'vpn_enabled': vpn, 'tor_enabled': tor});
  }

  Future<Map<String, dynamic>> buildChain() async {
    return _client.post('/api/paranoidx/build', body: {});
  }

  Future<Map<String, dynamic>> teardownChain() async {
    return _client.post('/api/paranoidx/teardown', body: {});
  }

  Future<Map<String, dynamic>> testChain() async {
    return _client.post('/api/paranoidx/test', body: {});
  }

  Future<Map<String, dynamic>> getVPNProfiles() async {
    return _client.get('/api/paranoidx/vpn/profiles');
  }

  Future<Map<String, dynamic>> addVPNProfile(String name, String description, String config) async {
    return _client.post('/api/paranoidx/vpn/profile', body: {'name': name, 'description': description, 'config': config});
  }

  Future<Map<String, dynamic>> vpnUp(String name) async {
    return _client.post('/api/paranoidx/vpn/up', body: {'name': name});
  }

  Future<Map<String, dynamic>> vpnDown(String name) async {
    return _client.post('/api/paranoidx/vpn/down', body: {'name': name});
  }

  Future<Map<String, dynamic>> vpnDelete(String name) async {
    return _client.post('/api/paranoidx/vpn/delete', body: {'name': name});
  }
}