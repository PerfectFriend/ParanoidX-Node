import 'dart:async';
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../theme.dart';
import '../main.dart';

class DashboardScreen extends StatefulWidget {
  const DashboardScreen({super.key});
  @override
  State<DashboardScreen> createState() => _DashboardScreenState();
}

class _DashboardScreenState extends State<DashboardScreen> {
  Map<String, dynamic>? _version, _health, _economy, _alerts, _ping;
  Timer? _timer;
  bool _isLoading = true;
  String? _error;

  @override
  void initState() {
    super.initState();
    _refresh();
    _timer = Timer.periodic(const Duration(seconds: 15), (_) => _refresh());
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }

  Future<void> _refresh() async {
    final api = context.read<AppState>().api;
    final appState = context.read<AppState>();
    try {
      final results = await Future.wait([
        api.getVersion(),
        api.getHealth(),
        api.getReserve(),
        api.getAlertRules(),
        api.ping(),
      ], eagerError: false);
      if (mounted) {
        setState(() {
          _version = results[0];
          _health = results[1];
          _economy = results[2];
          _alerts = results[3];
          _ping = results[4];
          _isLoading = false;
          _error = null;
        });
        appState.setOffline(false);
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _isLoading = false;
          _error = 'Connection lost: $e';
        });
        appState.setOffline(true);
      }
    }
  }

  String _safe(dynamic v, [String def = '—']) => v?.toString() ?? def;

  @override
  Widget build(BuildContext context) {
    final state = context.watch<AppState>();
    final live = state.liveTelemetry ?? {};
    final isOnline = _ping?['pong'] == true;
    return Scaffold(
      appBar: AppBar(
        title: const Text('Isle Royal Dashboard'),
        actions: [
          Container(
            margin: const EdgeInsets.only(right: 12),
            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
            decoration: BoxDecoration(
              color:
                  (isOnline ? RoyalTheme.green : RoyalTheme.red).withAlpha(30),
              borderRadius: BorderRadius.circular(20),
              border: Border.all(
                  color: (isOnline ? RoyalTheme.green : RoyalTheme.red)
                      .withAlpha(80)),
            ),
            child: Row(mainAxisSize: MainAxisSize.min, children: [
              Container(
                  width: 8,
                  height: 8,
                  decoration: BoxDecoration(
                      shape: BoxShape.circle,
                      color: isOnline ? RoyalTheme.green : RoyalTheme.red)),
              const SizedBox(width: 6),
              Text(isOnline ? 'Online' : 'Offline',
                  style: TextStyle(
                      fontSize: 14,
                      color: isOnline ? RoyalTheme.green : RoyalTheme.red)),
            ]),
          ),
        ],
      ),
      body: _isLoading
          ? const Center(child: CircularProgressIndicator(color: RoyalTheme.gold))
          : _error != null && _version == null
              ? _buildErrorView()
              : RefreshIndicator(
                  onRefresh: _refresh,
                  child: ListView(
                    padding: const EdgeInsets.all(20),
                    children: [
                      _buildBuildCard(),
                      const SizedBox(height: 16),
                      _buildLiveTelemetry(live),
                      const SizedBox(height: 16),
                      _buildHealthGrid(),
                      const SizedBox(height: 16),
                      _buildEconomyCard(),
                      const SizedBox(height: 16),
                      _buildAlertCard(),
                      if (_error != null) ...[
                        const SizedBox(height: 12),
                        _buildErrorBanner(),
                      ],
                    ],
                  ),
                ),
    );
  }

  Widget _buildErrorView() {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.cloud_off, size: 64, color: RoyalTheme.red),
            const SizedBox(height: 16),
            Text('Cannot connect to simplex-node',
                style: Theme.of(context).textTheme.titleLarge),
            const SizedBox(height: 8),
            Text(_error ?? 'Unknown error',
                textAlign: TextAlign.center,
                style: Theme.of(context).textTheme.bodyMedium),
            const SizedBox(height: 24),
            FilledButton.tonalIcon(
              onPressed: () {
                setState(() => _isLoading = true);
                _refresh();
              },
              icon: const Icon(Icons.refresh),
              label: const Text('Retry'),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildErrorBanner() {
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: RoyalTheme.red.withAlpha(20),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: RoyalTheme.red.withAlpha(60)),
      ),
      child: Row(children: [
        const Icon(Icons.warning_amber, color: RoyalTheme.red, size: 18),
        const SizedBox(width: 8),
        Expanded(
          child: Text(_error ?? '',
              style: const TextStyle(color: RoyalTheme.red, fontSize: 13)),
        ),
        GestureDetector(
          onTap: () {
            setState(() => _isLoading = true);
            _refresh();
          },
          child: const Icon(Icons.refresh, color: RoyalTheme.gold, size: 20),
        ),
      ]),
    );
  }

  Widget _buildBuildCard() {
    final ver = _version?['build'] ?? '—';
    final uptime = _health?['uptime_hours'] ?? 0;
    final started = _version?['started'] ?? '—';
    return Container(
      padding: const EdgeInsets.all(20),
      decoration: RoyalTheme.glowBorder(),
      child: Row(children: [
        Expanded(
            child:
                Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Text('Sovereign Node', style: Theme.of(context).textTheme.titleLarge),
          const SizedBox(height: 6),
          Text(_safe(ver),
              style: const TextStyle(
                  color: RoyalTheme.gold, fontWeight: FontWeight.w600)),
          const SizedBox(height: 2),
          Text('Started: $started',
              style: Theme.of(context).textTheme.bodyMedium),
        ])),
        Column(children: [
          Text('${uptime}h',
              style: const TextStyle(
                  fontSize: 36,
                  fontWeight: FontWeight.w700,
                  color: RoyalTheme.gold)),
          Text('uptime', style: Theme.of(context).textTheme.bodyMedium),
        ]),
      ]),
    );
  }

  Widget _buildLiveTelemetry(Map<String, dynamic> live) {
    if (live.isEmpty) return const SizedBox.shrink();
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        gradient: const LinearGradient(
            colors: [RoyalTheme.darkCard, Color(0xFF0D1B1C)]),
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: RoyalTheme.teal.withAlpha(60)),
      ),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Row(children: [
          Container(
              width: 8,
              height: 8,
              decoration: const BoxDecoration(
                  shape: BoxShape.circle, color: RoyalTheme.teal)),
          const SizedBox(width: 8),
          Text('Live Telemetry',
              style: Theme.of(context).textTheme.titleMedium),
          const Spacer(),
          Text('SSE',
              style: TextStyle(
                  fontSize: 13, color: RoyalTheme.teal.withAlpha(120))),
        ]),
        const SizedBox(height: 12),
        Row(children: [
          _liveMetric('Reserve', _safe(live['reserve_ng']), RoyalTheme.gold),
          const SizedBox(width: 16),
          _liveMetric('Supply', _safe(live['supply_ng']), RoyalTheme.silver),
          const SizedBox(width: 16),
          _liveMetric('Health', _safe(live['health']), RoyalTheme.green),
          const SizedBox(width: 16),
          _liveMetric('Tier', _safe(live['tier']), RoyalTheme.accent),
        ]),
        if (live['oracle_price'] != null) ...[
          const SizedBox(height: 8),
          Text('Silver: \$${live['oracle_price']}',
              style: const TextStyle(fontSize: 15, color: RoyalTheme.gold)),
        ],
      ]),
    );
  }

  Widget _liveMetric(String label, String value, Color color) {
    return Expanded(
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
      Text(value,
          style: TextStyle(
              fontSize: 16, fontWeight: FontWeight.w700, color: color)),
      Text(label,
          style: const TextStyle(fontSize: 13, color: Color(0xFF8B949E))),
    ]));
  }

  Widget _buildHealthGrid() {
    final h = _health ?? {};
    final bridgeOk = h['bridge'] == true || h['bridge_connected'] == true;
    final healthy = h['healthy'] ?? false;
    final msgCount = h['messages'] ?? h['message_count'] ?? h['total_messages'] ?? 0;
    return Row(children: [
      Expanded(
          child: _statCard(
              'Server',
              healthy == true ? 'Healthy' : 'Degraded',
              healthy == true ? RoyalTheme.green : RoyalTheme.orange,
              Icons.check_circle)),
      const SizedBox(width: 12),
      Expanded(
          child: _statCard('Bridge', bridgeOk ? 'Connected' : 'Disconnected',
              bridgeOk ? RoyalTheme.green : RoyalTheme.red, Icons.wifi)),
      const SizedBox(width: 12),
      Expanded(
          child: _statCard(
              'Messages', _safe(msgCount), RoyalTheme.accent, Icons.email)),
    ]);
  }

  Widget _statCard(String label, String value, Color color, IconData icon) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: RoyalTheme.gradientCard(),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Row(children: [
          Icon(icon, size: 18, color: color),
          const SizedBox(width: 6),
          Text(label, style: Theme.of(context).textTheme.bodyMedium),
        ]),
        const SizedBox(height: 8),
        Text(value,
            style: TextStyle(
                fontSize: 20, fontWeight: FontWeight.w700, color: color)),
      ]),
    );
  }

  Widget _buildEconomyCard() {
    final e = _economy ?? {};
    final reserve = e['reserve_ng'] ?? 0;
    final supply = e['supply_ng'] ?? 0;
    final price = e['silver_spot_usd'] ?? e['current_price'] ?? 0;
    final ratio = supply is num && supply > 0 && reserve is num
        ? '${(reserve / supply * 100).toStringAsFixed(1)}%'
        : '100%';
    return Container(
      padding: const EdgeInsets.all(20),
      decoration: RoyalTheme.gradientCard(),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Row(children: [
          const Icon(Icons.account_balance, size: 20, color: RoyalTheme.gold),
          const SizedBox(width: 8),
          Text('Treasury Overview',
              style: Theme.of(context).textTheme.titleLarge),
        ]),
        const SizedBox(height: 16),
        Row(children: [
          Expanded(child: _metric('Reserve', '$reserve ng', RoyalTheme.gold)),
          Expanded(child: _metric('Supply', '$supply ng', RoyalTheme.silver)),
          Expanded(child: _metric('Price', '\$$price', RoyalTheme.green)),
          Expanded(child: _metric('Backing', ratio, RoyalTheme.accent)),
        ]),
      ]),
    );
  }

  Widget _metric(String label, String value, Color color) {
    return Column(children: [
      Text(value,
          style: TextStyle(
              fontSize: 16, fontWeight: FontWeight.w700, color: color)),
      const SizedBox(height: 2),
      Text(label, style: Theme.of(context).textTheme.bodyMedium),
    ]);
  }

  Widget _buildAlertCard() {
    final rules = _alerts?['rules'] as List? ?? [];
    return Container(
      padding: const EdgeInsets.all(20),
      decoration: RoyalTheme.gradientCard(
          colors: [RoyalTheme.darkCard, const Color(0xFF1E1515)]),
      child: Row(children: [
        const Icon(Icons.notifications_active,
            color: RoyalTheme.orange, size: 24),
        const SizedBox(width: 12),
        Expanded(
            child:
                Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Text(
              '${rules.length} Alert Rule${rules.length == 1 ? '' : 's'} Active',
              style: Theme.of(context).textTheme.titleMedium),
          Text('Monitoring treasury, bridge & system health',
              style: Theme.of(context).textTheme.bodyMedium),
        ])),
        const Icon(Icons.chevron_right, color: RoyalTheme.gold),
      ]),
    );
  }
}
