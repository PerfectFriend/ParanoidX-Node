import 'simplex_api_client.dart';
import 'package:models/models.dart' show TreasuryState, ReserveState, MintResult, BurnResult, SystemStatus, HealthStatus, OraclePrice, DeflationState, AutoMintConfig, DividendPool, DividendHistory, Banknote, ProofOfReserve, Rates, Tokenomics, Forecast, NodeInfo, SystemMetrics, DockerStatus, ServiceStatus, Diagnostics, ServiceActionResult, BackupResult, CleanupResult, Config, MaintenanceMode, EmergencyStop, RateLimitStats, ContainerStatus, ContainerActionResult, Constitution, Proposal, ProposalDraft, VoteResult, DelegationResult, ChatStatus, Conversation, MessageResult, BroadcastResult, TreasuryAlertResult, ChatMessage;

class RoyalClient {
  final SimplexApiClient _client;
  RoyalClient(this._client);

  late final TreasuryApi treasury = TreasuryApi(_client);
  late final GovernanceApi governance = GovernanceApi(_client);
  late final SystemApi system = SystemApi(_client);
  late final EconomyApi economy = EconomyApi(_client);
  late final ChatApi chat = ChatApi(_client);

  Future<List<dynamic>> nodes() => _client.get('/api/royal/nodes').then((d) => d['nodes'] as List);
  Future<Map<String, dynamic>> register(String pubkey, String label, String addr) => _client.post('/api/royal/register', body: {'pubkey': pubkey, 'label': label, 'addr': addr});
  Future<Map<String, dynamic>> sendCommand(String command, String targetPubkey) => _client.post('/api/royal/command', body: {'command': command, 'target': targetPubkey});
  Future<Map<String, dynamic>> heartbeat(String pubkey) => _client.get('/api/royal/heartbeat', params: {'pubkey': pubkey});
  Future<Map<String, dynamic>> royalKey() => _client.get('/api/royal/key');
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

class EconomyApi {
  final SimplexApiClient _client;
  EconomyApi(this._client);

  Future<OraclePrice> oracle() async {
    final data = await _client.get('/api/economy/oracle');
    return OraclePrice.fromJson(data);
  }

  Future<OraclePrice> updateOracle(double price) async {
    final data = await _client.post('/api/economy/oracle', body: {'price': price});
    return OraclePrice.fromJson(data);
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
    return _client.events().where((e) => e['conversation_id'] == conversationId).map((e) => ChatMessage.fromJson(e));
  }
}