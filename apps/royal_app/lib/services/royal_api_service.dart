import 'dart:async';
import 'dart:convert';
import 'package:http/http.dart' as http;
import 'package:models/models.dart';

class RoyalApiService {
  static String defaultBaseUrl = 'http://127.0.0.1:8080';
  final String baseUrl;
  final http.Client _client = http.Client();
  static const Duration _requestTimeout = Duration(seconds: 15);
  Identity? _identity;

  RoyalApiService([String? url]) : baseUrl = url ?? defaultBaseUrl;

  Map<String, String> get _headers {
    final headers = <String, String>{
      'Content-Type': 'application/json',
      'Accept': 'application/json',
    };
    
    if (_identity != null) {
      headers['X-Identity-Pubkey'] = _identity!.ed25519PubKey;
      headers['X-Identity-Timestamp'] = DateTime.now().millisecondsSinceEpoch.toString();
    }
    
    return headers;
  }

  Future<void> setIdentity(Identity identity) async {
    _identity = identity;
  }

  Identity? get identity => _identity;

  Future<Map<String, dynamic>> _get(String path) async {
    final r = await _client
        .get(Uri.parse('$baseUrl$path'), headers: _headers)
        .timeout(_requestTimeout);
    if (r.statusCode != 200) throw ApiException(r.statusCode, r.body);
    return jsonDecode(r.body) as Map<String, dynamic>;
  }

  Future<Map<String, dynamic>> _post(String path, {Map<String, dynamic>? body}) async {
    final r = await _client
        .post(
          Uri.parse('$baseUrl$path'),
          headers: _headers,
          body: body != null ? jsonEncode(body) : null,
        )
        .timeout(_requestTimeout);
    if (r.statusCode != 200) throw ApiException(r.statusCode, r.body);
    return jsonDecode(r.body) as Map<String, dynamic>;
  }

  void dispose() {
    _client.close();
  }

  // ── Health & Version ──
  Future<Map<String, dynamic>> getVersion() => _get('/api/version');
  Future<Map<String, dynamic>> getHealth() => _get('/api/health');
  Future<Map<String, dynamic>> getHealthChecks() => _get('/api/health/checks');

  // ── Backend-API-compatible aliases (v1 paths mapped to actual endpoints) ──
  Future<Map<String, dynamic>> getStatusV1() => _get('/api/version');
  Future<Map<String, dynamic>> getEconomySummaryV1() => _get('/api/royal/treasury/state');
  Future<Map<String, dynamic>> getHealthV1() => _get('/api/health');
  Future<Map<String, dynamic>> getFullStatus() => _get('/api/status');
  Future<Map<String, dynamic>> getPingAdmin() => _get('/api/admin/ping');

  // ── AI Steward ──
  Future<Map<String, dynamic>> aiChat(String question, {String? context, String? profile, String? userId}) =>
      _post('/api/ai/chat', body: {'question': question, if (context != null) 'context': context, if (profile != null) 'profile': profile, if (userId != null) 'user_id': userId});

  Future<Map<String, dynamic>> aiExplainSilver() =>
      _post('/api/ai/explain-silver', body: {});
  Future<Map<String, dynamic>> aiMemoryStats() => _get('/api/ai/memory/stats');
  Future<Map<String, dynamic>> aiHealth() => _get('/api/ai/health');

  // ── Royal Treasury (corrected paths with /treasury/) ──
  Future<Map<String, dynamic>> getReserve() => _get('/api/royal/treasury/state');
  Future<Map<String, dynamic>> getReserveDetail() => _get('/api/royal/treasury/reserve');
  Future<Map<String, dynamic>> creditReserve(int amountNg, {String reason = 'manual'}) =>
      _post('/api/royal/treasury/reserve', body: {'amount_ng': amountNg, 'action': 'credit', 'reason': reason});
  Future<Map<String, dynamic>> debitReserve(int amountNg, {String reason = 'manual'}) =>
      _post('/api/royal/treasury/reserve', body: {'amount_ng': amountNg, 'action': 'debit', 'reason': reason});

  Future<Map<String, dynamic>> getSilverOracle() => _get('/api/royal/treasury/oracle');
  Future<Map<String, dynamic>> updateOracle(double price) =>
      _post('/api/royal/treasury/oracle', body: {'price': price});

  Future<Map<String, dynamic>> getDeflation() => _get('/api/royal/treasury/deflation');
  Future<Map<String, dynamic>> triggerDeflation() =>
      _post('/api/royal/treasury/deflation', body: {});

  Future<Map<String, dynamic>> getAutoMintConfig() => _get('/api/royal/treasury/auto-mint');
  Future<Map<String, dynamic>> updateAutoMint(bool enabled, int intervalMs) =>
      _post('/api/royal/treasury/auto-mint', body: {'enabled': enabled, 'interval_ms': intervalMs});

  Future<Map<String, dynamic>> triggerDividend({int poolNg = 0}) =>
      _post('/api/royal/treasury/dividend?action=trigger', body: {'pool_ng': poolNg});
  Future<Map<String, dynamic>> getDividendHistory({int limit = 20}) =>
      _get('/api/royal/treasury/dividend?limit=$limit');

  Future<Map<String, dynamic>> mint(int amount, {String holder = 'treasury', String reason = 'manual mint'}) =>
      _post('/api/royal/treasury/mint', body: {'holder': holder, 'amount_ng': amount, 'reason': reason});
  Future<Map<String, dynamic>> burn(String assetId, {String holder = 'treasury', String reason = 'manual burn'}) =>
      _post('/api/royal/treasury/burn', body: {'asset_id': assetId, 'holder': holder, 'reason': reason});

  Future<Map<String, dynamic>> getBanknotes() => _get('/api/royal/treasury/banknotes');
  Future<Map<String, dynamic>> getSilverAssets() => _get('/api/royal/treasury/silver-assets');
  Future<Map<String, dynamic>> getProofOfReserve() => _get('/api/royal/treasury/proof-of-reserve');
  Future<Map<String, dynamic>> getRates() => _get('/api/royal/treasury/rates');
  Future<Map<String, dynamic>> getTokenomics() => _get('/api/royal/treasury/tokenomics');
  Future<Map<String, dynamic>> getForecast() => _get('/api/royal/treasury/forecast');

  // ── Audit ──
  Future<Map<String, dynamic>> getAuditLog({int limit = 50, int offset = 0}) =>
      _get('/api/royal/audit-log?limit=$limit&offset=$offset');

  // ── Royal Dashboard & Alerts ──
  Future<Map<String, dynamic>> getAlertRules() => _get('/api/royal/alerts/list');
  Future<Map<String, dynamic>> addAlertRule(Map<String, dynamic> rule) =>
      _post('/api/royal/alerts/add', body: rule);
  Future<Map<String, dynamic>> deleteAlertRule(String id) =>
      _post('/api/royal/alerts/delete', body: {'id': id});

  // ── Multi-Sig ──
  Future<Map<String, dynamic>> getMultiSig() => _get('/api/royal/multisig');
  Future<Map<String, dynamic>> addSigner(String pubkey) =>
      _post('/api/royal/multisig', body: {'action': 'add', 'pubkey': pubkey});
  Future<Map<String, dynamic>> removeSigner(String pubkey) =>
      _post('/api/royal/multisig', body: {'action': 'remove', 'pubkey': pubkey});

  // ── Cron ──
  Future<Map<String, dynamic>> getCronRules() => _get('/api/royal/cron/list');
  Future<Map<String, dynamic>> addCronRule(Map<String, dynamic> rule) =>
      _post('/api/royal/cron/add', body: rule);
  Future<Map<String, dynamic>> deleteCronRule(String id) =>
      _post('/api/royal/cron/delete', body: {'id': id});

  // ── Sync ──
  Future<Map<String, dynamic>> getSync() => _get('/api/royal/sync');

  // ── Scheduled Actions ──
  Future<Map<String, dynamic>> getScheduledActions() => _get('/api/royal/schedule/list');
  Future<Map<String, dynamic>> createScheduledAction(Map<String, dynamic> action) =>
      _post('/api/royal/schedule/create', body: action);

  // ── Audit Export ──
  Future<Map<String, dynamic>> exportAudit({String format = 'json'}) =>
      _get('/api/royal/audit/export?format=$format');

  // ── Node Groups ──
  Future<Map<String, dynamic>> getNodeGroups() => _get('/api/royal/nodes/groups');
  Future<Map<String, dynamic>> createNodeGroup(String name, List<String> nodes) =>
      _post('/api/royal/nodes/groups', body: {'action': 'create', 'name': name, 'nodes': nodes});
  Future<Map<String, dynamic>> deleteNodeGroup(String name) =>
      _post('/api/royal/nodes/groups', body: {'action': 'delete', 'name': name});

  // ── Emergency Stop ──
  Future<Map<String, dynamic>> getEmergencyStop() => _get('/api/royal/emergency-stop');
  Future<Map<String, dynamic>> setEmergencyStop(bool enable) =>
      _post('/api/royal/emergency-stop', body: {'action': enable ? 'enable' : 'disable'});

  // ── Node Reputation ──
  Future<Map<String, dynamic>> getReputations() => _get('/api/royal/nodes/reputation');
  Future<Map<String, dynamic>> sendHeartbeat(String pubkey, {double latencyMs = 0}) =>
      _get('/api/royal/nodes/heartbeat?pubkey=$pubkey&latency_ms=$latencyMs');

  // ── Analytics ──
  Future<Map<String, dynamic>> getTreasuryTrends({int days = 30}) =>
      _get('/api/royal/analytics/treasury-trends?days=$days');

  // ── Crypto Reserves ──
  Future<Map<String, dynamic>> getCryptoReserves() => _get('/api/royal/crypto-reserves');
  Future<Map<String, dynamic>> updateCryptoReserve(Map<String, dynamic> reserve) =>
      _post('/api/royal/crypto-reserves', body: reserve);

  // ── Rate Limit Stats ──
  Future<Map<String, dynamic>> getRateLimitStats() => _get('/api/royal/rate-limit-stats');

  // ── Ping ──
  Future<Map<String, dynamic>> ping() => _get('/api/royal/test/ping');

  // ── DC Cloud ──
  Future<Map<String, dynamic>> dcStatus() => _get('/api/dc/status');
  Future<Map<String, dynamic>> dcList() => _get('/api/dc/list');
  Future<Map<String, dynamic>> dcSwarm() => _get('/api/dc/swarm');
  Future<Map<String, dynamic>> dcSeed(String infohash) =>
      _post('/api/dc/seed', body: {'infohash': infohash});

  // ── Chat Bridge ──
  Future<Map<String, dynamic>> chatStatus() => _get('/api/chat/status');
  Future<Map<String, dynamic>> chatBroadcast(String message) =>
      _post('/api/royal/chat/broadcast', body: {'message': message});
  Future<Map<String, dynamic>> chatTreasuryAlert() =>
      _post('/api/royal/chat/treasury-alert', body: {});

  // ── DC Cloud ──
  // ── Governance (corrected paths) ──
  Future<Map<String, dynamic>> getConstitution() => _get('/api/royal/governance/constitution');
  Future<Map<String, dynamic>> createProposal(Map<String, dynamic> proposal) =>
      _post('/api/royal/governance/proposals', body: proposal);
  Future<Map<String, dynamic>> getProposals() => _get('/api/royal/governance/proposals');

  // ── Economy ──
  Future<Map<String, dynamic>> getEconomyReport() => _get('/api/royal/treasury/state');
  Future<Map<String, dynamic>> getRates2() => _get('/api/economy/rates');
  Future<Map<String, dynamic>> getTreasuryForecast() => _get('/api/economy/treasury-forecast');

  // ── System ──
  Future<Map<String, dynamic>> getInfo() => _get('/api/admin/info');
  Future<Map<String, dynamic>> getSystemMetrics() => _get('/api/admin/metrics/system');
  Future<Map<String, dynamic>> getDockerStatus() => _get('/api/admin/docker');
  Future<Map<String, dynamic>> getServiceStatus() => _get('/api/admin/service/status');
  Future<Map<String, dynamic>> getDiagnostics() => _get('/api/admin/diagnostics');

  // ── Admin Management ──
  Future<Map<String, dynamic>> restartService(String service) =>
      _post('/api/admin/service/restart', body: {'service': service});
  Future<Map<String, dynamic>> triggerBackup() =>
      _post('/api/admin/backup', body: {});
  Future<Map<String, dynamic>> diskCleanup() =>
      _post('/api/admin/disk-cleanup', body: {});
  Future<Map<String, dynamic>> getConfig() => _get('/api/admin/config');
  Future<Map<String, dynamic>> updateConfig(Map<String, dynamic> cfg) =>
      _post('/api/admin/config', body: cfg);
  Future<Map<String, dynamic>> getMaintenance() => _get('/api/admin/maintenance');
  Future<Map<String, dynamic>> setMaintenance(bool active, {String message = ''}) =>
      _post('/api/admin/maintenance', body: {'active': active, 'message': message});
  Future<Map<String, dynamic>> getContainerList() => _get('/api/admin/container/list');
  Future<Map<String, dynamic>> getContainerLogs(String name, {int tail = 50}) =>
      _get('/api/admin/container/logs?name=$name&tail=$tail');
  Future<Map<String, dynamic>> getDiskUsage() => _get('/api/admin/disk-usage');
  Future<Map<String, dynamic>> getDiskTrend({int hours = 24}) =>
      _get('/api/admin/disk-trend?hours=$hours');
  Future<Map<String, dynamic>> getBandwidthMetrics() => _get('/api/admin/metrics/bandwidth');
  Future<Map<String, dynamic>> getMemoryTrend() => _get('/api/admin/metrics/memory-trend');
  Future<Map<String, dynamic>> getPerfMetrics() => _get('/api/admin/metrics/perf');
  Future<Map<String, dynamic>> resetPerfMetrics() =>
      _post('/api/admin/metrics/perf/reset', body: {});
  Future<Map<String, dynamic>> verifyBackup() =>
      _post('/api/admin/backup/verify', body: {});

  // ── Container ──
  Future<Map<String, dynamic>> getContainerStatus() => _get('/api/container/status');
  Future<Map<String, dynamic>> openContainer(String password) =>
      _post('/api/container/open', body: {'password': password});
  Future<Map<String, dynamic>> closeContainer() =>
      _post('/api/container/close', body: {});

  // ── ParanoidX ──
  Future<Map<String, dynamic>> getParanoidXStatus() => _get('/api/paranoidx/status');
  Future<Map<String, dynamic>> getParanoidXHistory() => _get('/api/paranoidx/history');
  Future<Map<String, dynamic>> getParanoidXConfig() => _get('/api/paranoidx/config');
  Future<Map<String, dynamic>> paranoidXChainBuild() =>
      _post('/api/paranoidx/chain/build', body: {});
  Future<Map<String, dynamic>> paranoidXChainTeardown() =>
      _post('/api/paranoidx/chain/teardown', body: {});

  // ── SSE Events ──
  Stream<Map<String, dynamic>> _sseStream(String path) {
    late StreamController<Map<String, dynamic>> controller;
    controller = StreamController<Map<String, dynamic>>(
      onCancel: () => controller.close(),
    );
    final request = http.Request('GET', Uri.parse('$baseUrl$path'));
    request.headers.addAll(_headers);
    _client.send(request).then((response) {
      response.stream
          .transform(utf8.decoder)
          .transform(const LineSplitter())
          .listen((line) {
        if (line.startsWith('data: ')) {
          try {
            final data = jsonDecode(line.substring(6)) as Map<String, dynamic>;
            controller.add(data);
          } catch (_) {}
        }
      }, onError: (e) => controller.addError(e));
    });
    return controller.stream;
  }

  Stream<Map<String, dynamic>> sseEvents() => _sseStream('/api/royal/events');
  Stream<Map<String, dynamic>> sseAdminEvents() => _sseStream('/api/admin/events');

  // ── PANIC Wipe ──
  Future<Map<String, dynamic>> panicWipe() =>
      _post('/api/panic', body: {});

  // ── Radio Management ──
  Future<Map<String, dynamic>> getRadioAI() => _get('/api/radio/ai-content');
  Future<Map<String, dynamic>> postRadioAI(Map<String, dynamic> body) =>
      _post('/api/radio/ai-content', body: body);
  Future<Map<String, dynamic>> getRadioSchedule() => _get('/api/radio/schedule');
  Future<Map<String, dynamic>> getRadioScheduleContent() => _get('/api/radio/schedule-content');
  Future<Map<String, dynamic>> postRadioScheduleContent(Map<String, dynamic> body) =>
      _post('/api/radio/schedule-content', body: body);

  // ── Steward Rules & Tasks ──
  Future<Map<String, dynamic>> getAutoReplyRules() => _get('/api/chat/auto-reply');
  Future<Map<String, dynamic>> addAutoReplyRule(Map<String, dynamic> rule) =>
      _post('/api/chat/auto-reply', body: rule);
  Future<Map<String, dynamic>> deleteAutoReplyRule(String id) =>
      _post('/api/chat/auto-reply/delete', body: {'id': id});
  Future<Map<String, dynamic>> getStewardMonitor() => _get('/api/ai/monitor');
  Future<Map<String, dynamic>> getStewardConstitution() => _get('/api/ai/constitution');
  Future<Map<String, dynamic>> aiSuggestTreasury() =>
      _post('/api/ai/suggest-treasury', body: {});
  Future<Map<String, dynamic>> aiEconomySummary() =>
      _post('/api/ai/economy-summary', body: {});

  // ── Moderation ──
  Future<Map<String, dynamic>> getModerationStats() => _get('/api/admin/moderation-stats');

  // ── Admin Audit Log ──
  Future<Map<String, dynamic>> getAdminAuditLog({int limit = 50, int offset = 0}) =>
      _get('/api/admin/audit-log?limit=$limit&offset=$offset');
}

class ApiException implements Exception {
  final int statusCode;
  final String body;
  ApiException(this.statusCode, this.body);

  String get message => 'API error $statusCode: $body';
  @override String toString() => message;
}
