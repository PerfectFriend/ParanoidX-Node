import 'dart:async';
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../theme.dart';
import '../main.dart';

class SystemScreen extends StatefulWidget {
  const SystemScreen({super.key});
  @override
  State<SystemScreen> createState() => _SystemScreenState();
}

class _SystemScreenState extends State<SystemScreen> {
  Map<String, dynamic>? _health,
      _checks,
      _metrics,
      _docker,
      _paranoidx,
      _fullStatus;
  Timer? _timer;
  bool _loading = true;

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
    try {
      final r = await Future.wait([
        api.getHealth(),
        api.getHealthChecks(),
        api.getSystemMetrics(),
        api.getDockerStatus(),
        api.getParanoidXStatus(),
        api.getFullStatus(),
      ], eagerError: false);
      if (mounted) {
        setState(() {
          _health = r[0];
          _checks = r[1];
          _metrics = r[2];
          _docker = r[3];
          _paranoidx = r[4];
          _fullStatus = r[5];
          _loading = false;
        });
      }
    } catch (e) {
      if (mounted) {
        setState(() => _loading = false);
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(
            content: Text('Refresh failed: $e'),
            backgroundColor: RoyalTheme.red.withAlpha(40)));
      }
    }
  }

  String _safe(dynamic v, [String def = '—']) => v?.toString() ?? def;

  String _pct(Map<String, dynamic>? m, String key) {
    if (m == null) return '—';
    final v = m[key];
    if (v is num) return '${v.toStringAsFixed(1)}%';
    if (v is String) return v;
    return '—';
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return const Scaffold(body: Center(child: CircularProgressIndicator()));
    }
    return Scaffold(
      appBar: AppBar(
        title: Row(children: [
          Container(
              padding: const EdgeInsets.all(6),
              decoration: BoxDecoration(
                  color: RoyalTheme.accent.withAlpha(30),
                  borderRadius: BorderRadius.circular(8)),
              child: const Icon(Icons.monitor_heart,
                  color: RoyalTheme.accent, size: 20)),
          const SizedBox(width: 12),
          const Text('System'),
        ]),
        actions: [
          IconButton(icon: const Icon(Icons.refresh), onPressed: _refresh)
        ],
      ),
      body: RefreshIndicator(
        onRefresh: _refresh,
        child: ListView(
          padding: const EdgeInsets.all(16),
          children: [
            _healthOverview(),
            const SizedBox(height: 16),
            _metricsGrid(),
            const SizedBox(height: 16),
            _dockerCard(),
            const SizedBox(height: 16),
            _paranoidxCard(),
            const SizedBox(height: 16),
            _serviceDetails(),
            const SizedBox(height: 16),
            _checkList(),
          ],
        ),
      ),
    );
  }

  Widget _healthOverview() {
    final h = _health ?? {};
    final fs = _fullStatus ?? {};
    final healthy = h['healthy'] == true;
    final uptime = h['uptime_hours'] ?? 0;
    final br = fs['bridge'];
    final bridgeOk =
        h['bridge'] == true || (br is Map && br['connected'] == true);
    final mem = fs['memory'];
    final load = fs['load'];
    return Container(
      padding: const EdgeInsets.all(20),
      decoration: RoyalTheme.glowBorder(
          color: healthy ? RoyalTheme.green : RoyalTheme.orange),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Row(children: [
          Container(
              width: 48,
              height: 48,
              decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: (healthy ? RoyalTheme.green : RoyalTheme.orange)
                      .withAlpha(30)),
              child: Icon(healthy ? Icons.check_circle : Icons.warning,
                  color: healthy ? RoyalTheme.green : RoyalTheme.orange,
                  size: 28)),
          const SizedBox(width: 16),
          Expanded(
              child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                Text(healthy ? 'All Systems Operational' : 'System Degraded',
                    style: Theme.of(context).textTheme.titleMedium),
                Text('Uptime: ${uptime}h | Bridge: ${bridgeOk ? 'OK' : 'DOWN'}',
                    style: Theme.of(context).textTheme.bodyMedium),
              ])),
        ]),
        const SizedBox(height: 12),
        _kv('Version', _safe(fs['version'] ?? h['build'])),
        _kv('Status', _safe(fs['status'], 'running')),
        _kv(
            'Memory',
            mem is Map
                ? '${_safe(mem['used_mb'])} MB / ${_safe(mem['total_mb'])} MB'
                : '—'),
        _kv(
            'Load',
            load is Map
                ? '${_safe(load['load1'])} / ${_safe(load['load5'])} / ${_safe(load['load15'])}'
                : '—'),
        _kv('Goroutines', _safe(_metrics?['goroutines'])),
      ]),
    );
  }

  Widget _metricsGrid() {
    final m = _metrics ?? {};
    final cpu = m['cpu_percent'] is num
        ? '${(m['cpu_percent'] as num).toStringAsFixed(1)}%'
        : _safe(m['cpu_percent']);
    final ram = _pct(
        m['ram'] is Map ? m['ram'] as Map<String, dynamic>? : null, 'used_pct');
    final disk = _pct(
        m['disk'] is Map ? m['disk'] as Map<String, dynamic>? : null,
        'used_pct');
    return Row(children: [
      Expanded(child: _metricCard('CPU', cpu, RoyalTheme.accent, Icons.memory)),
      const SizedBox(width: 12),
      Expanded(child: _metricCard('RAM', ram, RoyalTheme.green, Icons.storage)),
      const SizedBox(width: 12),
      Expanded(
          child: _metricCard('Disk', disk, RoyalTheme.orange, Icons.disc_full)),
    ]);
  }

  Widget _metricCard(String label, String value, Color color, IconData icon) {
    return Container(
      padding: const EdgeInsets.all(14),
      decoration: RoyalTheme.gradientCard(),
      child: Column(children: [
        Icon(icon, color: color, size: 20),
        const SizedBox(height: 6),
        Text(value,
            style: TextStyle(
                fontSize: 18, fontWeight: FontWeight.w700, color: color)),
        Text(label, style: Theme.of(context).textTheme.bodyMedium),
      ]),
    );
  }

  Widget _dockerCard() {
    final d = _docker ?? {};
    final containers = d['containers'];
    List<MapEntry<String, String>> entries = [];
    if (containers is Map) {
      for (final e in (containers as Map<String, dynamic>).entries) {
        entries.add(MapEntry(e.key, e.value?.toString() ?? '?'));
      }
    } else if (containers is List) {
      for (final c in containers) {
        if (c is Map) {
          entries.add(MapEntry(_safe(c['name'] ?? c['service']),
              _safe(c['state'] ?? c['status'])));
        }
      }
    }
    final running = entries.where((e) => e.value.startsWith('Up')).length;
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: RoyalTheme.gradientCard(),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Row(children: [
          const Icon(Icons.view_in_ar, color: RoyalTheme.accent, size: 18),
          const SizedBox(width: 8),
          Text('Docker ($running/${entries.length} running)',
              style: Theme.of(context).textTheme.titleMedium),
        ]),
        const SizedBox(height: 12),
        if (entries.isEmpty)
          Text('No containers', style: Theme.of(context).textTheme.bodyMedium)
        else
          ...entries.map((e) {
            final ok = e.value.startsWith('Up');
            return Padding(
                padding: const EdgeInsets.symmetric(vertical: 3),
                child: Row(children: [
                  Container(
                      width: 8,
                      height: 8,
                      decoration: BoxDecoration(
                          shape: BoxShape.circle,
                          color: ok ? RoyalTheme.green : RoyalTheme.red)),
                  const SizedBox(width: 8),
                  Text(e.key, style: const TextStyle(fontSize: 14)),
                  const Spacer(),
                  Text(e.value,
                      style: TextStyle(
                          fontSize: 13,
                          color: ok ? RoyalTheme.green : RoyalTheme.red)),
                ]));
          }),
      ]),
    );
  }

  Widget _paranoidxCard() {
    final p = _paranoidx ?? {};
    final layers = p['layers'] is List ? p['layers'] as List : [];
    final overall = p['overall_healthy'] == true;
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: RoyalTheme.glowBorder(color: RoyalTheme.purple),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Row(children: [
          const Icon(Icons.shield, color: RoyalTheme.purple, size: 18),
          const SizedBox(width: 8),
          Text('ParanoidX Security',
              style: Theme.of(context).textTheme.titleMedium),
          const Spacer(),
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
            decoration: BoxDecoration(
                color: (overall ? RoyalTheme.green : RoyalTheme.orange)
                    .withAlpha(30),
                borderRadius: BorderRadius.circular(10)),
            child: Text(overall ? 'SECURE' : 'DEGRADED',
                style: TextStyle(
                    fontSize: 13,
                    color: overall ? RoyalTheme.green : RoyalTheme.orange)),
          ),
        ]),
        const SizedBox(height: 12),
        if (layers.isEmpty)
          Text('No layer data', style: Theme.of(context).textTheme.bodyMedium)
        else
          ...layers.map((l) {
            if (l is! Map) return const SizedBox.shrink();
            final ok = l['healthy'] == true ||
                l['status'] == 'healthy' ||
                l['status'] == 'ok';
            final name = l['layer'] ?? l['name'] ?? '—';
            final detail = l['message'] ?? l['detail'] ?? '';
            return Padding(
                padding: const EdgeInsets.symmetric(vertical: 3),
                child: Row(children: [
                  Icon(ok ? Icons.check_circle : Icons.error,
                      size: 14, color: ok ? RoyalTheme.green : RoyalTheme.red),
                  const SizedBox(width: 8),
                  Expanded(
                      child:
                          Text('$name', style: const TextStyle(fontSize: 14))),
                  Text('$detail',
                      style: TextStyle(
                          fontSize: 13,
                          color: ok ? RoyalTheme.green : RoyalTheme.red)),
                ]));
          }),
      ]),
    );
  }

  Widget _serviceDetails() {
    final fs = _fullStatus ?? {};
    final smp = fs['smp'];
    final xftp = fs['xftp'];
    final vault = fs['vault'];
    final disk = fs['disk'];
    if (smp == null && xftp == null) return const SizedBox.shrink();
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: RoyalTheme.gradientCard(),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Row(children: [
          const Icon(Icons.info_outline, color: RoyalTheme.gold, size: 18),
          const SizedBox(width: 8),
          Text('Service Details',
              style: Theme.of(context).textTheme.titleMedium),
        ]),
        const SizedBox(height: 12),
        if (smp is Map) ...[
          _kv('SMP Status', _safe(smp['status'])),
          _kv('SMP Fingerprint', _safe(smp['fingerprint'])),
        ],
        if (xftp is Map) ...[
          _kv('XFTP Status', _safe(xftp['status'])),
          _kv('XFTP Fingerprint', _safe(xftp['fingerprint'])),
        ],
        if (vault is Map)
          _kv('Vault',
              '${_safe(vault['used_mb'])} MB / ${_safe(vault['quota_mb'])} MB (${_safe(vault['file_count'])} files)'),
        if (disk is Map) ...[
          if (disk['root'] is Map)
            _kv('Disk Root', _safe((disk['root'] as Map)['used_pct'])),
          if (disk['data'] is Map)
            _kv('Disk Data', _safe((disk['data'] as Map)['used_pct'])),
        ],
      ]),
    );
  }

  Widget _checkList() {
    final raw = _checks?['checks'];
    if (raw is! List) return const SizedBox.shrink();
    final checks = raw;
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: RoyalTheme.gradientCard(),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Text('Health Checks (${checks.length})',
            style: Theme.of(context).textTheme.titleMedium),
        const SizedBox(height: 12),
        ...checks.map((c) {
          if (c is! Map) return const SizedBox.shrink();
          final status = c['status']?.toString() ?? 'ok';
          final ok = status == 'ok' || c['healthy'] == true;
          return Padding(
              padding: const EdgeInsets.symmetric(vertical: 3),
              child: Row(children: [
                Icon(ok ? Icons.check_circle_outline : Icons.error_outline,
                    size: 14, color: ok ? RoyalTheme.green : RoyalTheme.red),
                const SizedBox(width: 8),
                Expanded(
                    child: Text('${c['name'] ?? '—'}',
                        style: const TextStyle(fontSize: 14))),
                Text(status,
                    style: TextStyle(
                        fontSize: 13,
                        color: ok ? RoyalTheme.green : RoyalTheme.red)),
              ]));
        }),
      ]),
    );
  }

  Widget _kv(String k, String v) {
    return Padding(
        padding: const EdgeInsets.symmetric(vertical: 2),
        child: Row(children: [
          Text('$k: ',
              style: const TextStyle(color: Color(0xFF8B949E), fontSize: 13)),
          Expanded(
              child: Text(v,
                  style: const TextStyle(color: Colors.white, fontSize: 13))),
        ]));
  }
}
