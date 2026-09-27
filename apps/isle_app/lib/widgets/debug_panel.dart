import 'dart:async';
import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:http/http.dart' as http;

class DebugPanel extends StatefulWidget {
  final String apiBase;
  const DebugPanel({super.key, required this.apiBase});
  @override
  State<DebugPanel> createState() => _DebugPanelState();
}

class _DebugPanelState extends State<DebugPanel> {
  Map<String, dynamic>? _data;
  Map<String, dynamic>? _checks;
  bool _loading = true;
  String? _error;
  Timer? _timer;

  @override
  void initState() {
    super.initState();
    _fetch();
    _timer = Timer.periodic(const Duration(seconds: 10), (_) => _fetch());
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }

  Future<void> _fetch() async {
    try {
      final r = await Future.wait([
        http.Client().get(Uri.parse('${widget.apiBase}/api/debug')).timeout(const Duration(seconds: 5)),
        http.Client().get(Uri.parse('${widget.apiBase}/api/health/checks')).timeout(const Duration(seconds: 5)),
      ]);
      if (mounted) setState(() {
        _data = jsonDecode(r[0].body) as Map<String, dynamic>;
        _checks = jsonDecode(r[1].body) as Map<String, dynamic>;
        _loading = false;
        _error = null;
      });
    } catch (e) {
      if (mounted) setState(() { _error = e.toString(); _loading = false; });
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) return const Center(child: CircularProgressIndicator());
    if (_error != null) return Center(child: Text('Error: $_error', style: const TextStyle(color: Color(0xFFF85149))));
    return ListView(padding: const EdgeInsets.all(16), children: [
      _section('System', Icons.memory, _buildSystem()),
      _section('ParanoidX Proxy Chain', Icons.shield, _buildParanoidX()),
      _section('Transport Hub', Icons.wifi, _buildTransport()),
      _section('Bridge', Icons.link, _buildBridge()),
      _section('Health Checks', Icons.monitor_heart, _buildHealthChecks()),
    ]);
  }

  Widget _section(String title, IconData icon, Widget child) {
    return Container(
      margin: const EdgeInsets.only(bottom: 16),
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: const Color(0xFF161B22),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: const Color(0xFF30363D)),
      ),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Row(children: [
          Icon(icon, size: 20, color: const Color(0xFF58A6FF)),
          const SizedBox(width: 8),
          Text(title, style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w700, color: Color(0xFFFFD700))),
        ]),
        const SizedBox(height: 12),
        child,
      ]),
    );
  }

  Widget _row(String label, String value, {Color? valueColor}) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 3),
      child: Row(children: [
        Text('$label: ', style: const TextStyle(fontSize: 14, color: Color(0xFF8B949E))),
        Expanded(child: Text(value, style: TextStyle(fontSize: 14, fontWeight: FontWeight.w600, color: valueColor ?? Colors.white))),
      ]),
    );
  }

  Widget _badge(String text, bool ok) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
      decoration: BoxDecoration(
        color: (ok ? const Color(0xFF166534) : const Color(0xFF7F1D1D)).withAlpha(180),
        borderRadius: BorderRadius.circular(6),
      ),
      child: Text(text, style: TextStyle(fontSize: 12, fontWeight: FontWeight.w700, color: ok ? const Color(0xFF4ADE80) : const Color(0xFFF87171))),
    );
  }

  Widget _buildSystem() {
    final sys = _data?['system'] as Map<String, dynamic>? ?? {};
    final ram = sys['ram'] as Map<String, dynamic>? ?? {};
    final disk = sys['disk'] as Map<String, dynamic>? ?? {};
    final mem = sys['memory'] as Map<String, dynamic>? ?? {};
    final cpu = (sys['cpu_percent'] as num?)?.toDouble() ?? 0;
    return Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
      Row(children: [
        _gauge('CPU', cpu),
        const SizedBox(width: 12),
        _gauge('RAM', double.tryParse((ram['used_pct'] as String? ?? '0').replaceAll('%', '')) ?? 0),
        const SizedBox(width: 12),
        _gauge('DISK', double.tryParse((disk['used_pct'] as String? ?? '0').replaceAll('%', '')) ?? 0),
      ]),
      const SizedBox(height: 12),
      _row('Go', '${sys['go_version'] ?? '?'} · ${sys['goroutines'] ?? 0} goroutines · ${sys['cpus'] ?? 0} CPUs'),
      _row('Uptime', sys['uptime_human'] ?? '${sys['uptime_seconds'] ?? 0}s'),
      _row('Memory', 'Alloc ${mem['alloc_mb']}MB · Heap ${mem['heap_mb']}MB · GC ${mem['num_gc']} cycles'),
    ]);
  }

  Widget _gauge(String label, double pct) {
    final color = pct > 90 ? const Color(0xFFF87171) : pct > 70 ? const Color(0xFFFBBF24) : const Color(0xFF4ADE80);
    return Expanded(
      child: Container(
        padding: const EdgeInsets.all(12),
        decoration: BoxDecoration(
          color: const Color(0xFF0D1117),
          borderRadius: BorderRadius.circular(10),
          border: Border.all(color: color.withAlpha(60)),
        ),
        child: Column(children: [
          Text('${pct.toStringAsFixed(0)}%', style: TextStyle(fontSize: 26, fontWeight: FontWeight.w700, color: color)),
          Text(label, style: const TextStyle(fontSize: 12, color: Color(0xFF8B949E), fontWeight: FontWeight.w600)),
          const SizedBox(height: 6),
          ClipRRect(
            borderRadius: BorderRadius.circular(4),
            child: LinearProgressIndicator(
              value: (pct / 100).clamp(0.0, 1.0),
              backgroundColor: const Color(0xFF21262D),
              valueColor: AlwaysStoppedAnimation(color),
              minHeight: 6,
            ),
          ),
        ]),
      ),
    );
  }

  Widget _buildParanoidX() {
    final px = _data?['paranoidx'] as Map<String, dynamic>? ?? {};
    final layers = px['layers'] as List<dynamic>? ?? [];
    final ok = px['overall_healthy'] == true;
    return Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
      Row(children: [
        Text('Proxy Chain', style: const TextStyle(fontSize: 14, color: Color(0xFF8B949E))),
        const SizedBox(width: 8),
        _badge(ok ? 'HEALTHY' : 'DEGRADED', ok),
      ]),
      const SizedBox(height: 8),
      ...layers.map((l) {
        final m = l as Map<String, dynamic>;
        final healthy = m['healthy'] == true;
        return Padding(
          padding: const EdgeInsets.symmetric(vertical: 3),
          child: Row(children: [
            Icon(healthy ? Icons.check_circle : Icons.error, size: 16, color: healthy ? const Color(0xFF4ADE80) : const Color(0xFFF87171)),
            const SizedBox(width: 8),
            Text('${m['layer']}', style: const TextStyle(fontSize: 14, fontWeight: FontWeight.w600, color: Colors.white)),
            const SizedBox(width: 8),
            Text('${m['message'] ?? ''}', style: const TextStyle(fontSize: 12, color: Color(0xFF8B949E))),
            const Spacer(),
            Text('${m['latency_ms'] ?? 0}ms', style: const TextStyle(fontSize: 12, color: Color(0xFF8B949E))),
          ]),
        );
      }),
    ]);
  }

  Widget _buildTransport() {
    final ts = _data?['transport']?['stats'] as Map<String, dynamic>? ?? {};
    return Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
      _row('Registered Apps', '${ts['registered_apps'] ?? 0}'),
      _row('WebSocket Connections', '${ts['active_ws_connections'] ?? 0}'),
      _row('Health Score', '${ts['health_score'] ?? 0}/${ts['health'] ?? '?'}'),
      _row('Messages Sent', '${ts['stats']?['messages_sent'] ?? 0}'),
      _row('Messages Received', '${ts['stats']?['messages_received'] ?? 0}'),
      _row('Auth Failures', '${ts['stats']?['auth_failures'] ?? 0}'),
      _row('Rate Limit Hits', '${ts['stats']?['rate_limit_hits'] ?? 0}'),
    ]);
  }

  Widget _buildBridge() {
    final br = _data?['bridge'] as Map<String, dynamic>? ?? {};
    final connected = br['connected'] == true;
    return Row(children: [
      Icon(connected ? Icons.check_circle : Icons.error, size: 20, color: connected ? const Color(0xFF4ADE80) : const Color(0xFFF87171)),
      const SizedBox(width: 8),
      Text(connected ? 'Bridge Connected' : 'Bridge Disconnected',
          style: TextStyle(fontSize: 16, fontWeight: FontWeight.w700, color: connected ? const Color(0xFF4ADE80) : const Color(0xFFF87171))),
      const Spacer(),
      Text('Reconnects: ${br['reconnect_count'] ?? 0}', style: const TextStyle(fontSize: 14, color: Color(0xFF8B949E))),
    ]);
  }

  Widget _buildHealthChecks() {
    final checks = _checks?['checks'] as List<dynamic>? ?? [];
    return Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
      ...checks.take(15).map((c) {
        final m = c as Map<String, dynamic>;
        final st = m['status'] ?? 'ok';
        final ok = st == 'ok';
        return Padding(
          padding: const EdgeInsets.symmetric(vertical: 2),
          child: Row(children: [
            Icon(ok ? Icons.check_circle : Icons.warning, size: 14,
                color: ok ? const Color(0xFF4ADE80) : (st == 'warn' ? const Color(0xFFFBBF24) : const Color(0xFFF87171))),
            const SizedBox(width: 6),
            Expanded(
              child: Text('${m['name']}', style: const TextStyle(fontSize: 13, color: Color(0xFFC9D1D9))),
            ),
          ]),
        );
      }),
      if (checks.length > 15)
        Padding(
          padding: const EdgeInsets.only(top: 4),
          child: Text('+ ${checks.length - 15} more', style: const TextStyle(fontSize: 12, color: Color(0xFF484F58))),
        ),
    ]);
  }
}
