import 'dart:async';
import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:http/http.dart' as http;

enum _MState { pending, busy, ok, fail, stuck, revive }

class _Metric {
  final String name;
  final String path;
  final IconData icon;
  _MState state = _MState.pending;
  dynamic value;
  String text = '—';
  int fails = 0;

  _Metric(this.name, this.path, this.icon);

  Color get color {
    switch (state) {
      case _MState.ok:    return const Color(0xFF3FB950);
      case _MState.fail:  return const Color(0xFFF85149);
      case _MState.stuck: return const Color(0xFFFF6B6B);
      case _MState.revive:return const Color(0xFFD29922);
      default: return const Color(0xFF8B949E);
    }
  }

  String get badge {
    switch (state) {
      case _MState.pending: return '⏳';
      case _MState.busy:    return '🔄';
      case _MState.ok:      return '✓';
      case _MState.fail:    return '✗';
      case _MState.stuck:   return '🔴';
      case _MState.revive:  return '💊';
    }
  }
}

/// MetricsPanel manages a live telemetry panel that polls system metrics from the server.
class MetricsPanel extends StatefulWidget {
  final String apiBase;
  const MetricsPanel({super.key, required this.apiBase});
  @override
  State<MetricsPanel> createState() => MetricsPanelState();
}

/// MetricsPanelState manages a live telemetry panel that polls system metrics from the server.
class MetricsPanelState extends State<MetricsPanel> {
  final List<_Metric> _all = [];
  bool _ready = false;
  bool _polling = false;

  @override
  void initState() {
    super.initState();
    _all.addAll([
      _Metric('CPU',    '/api/admin/metrics/system', Icons.memory),
      _Metric('RAM',    '/api/admin/metrics/system', Icons.storage),
      _Metric('Disk',   '/api/admin/metrics/system', Icons.disc_full),
      _Metric('Bridge', '/api/chat/status',          Icons.wifi),
      _Metric('Node',   '/api/version',              Icons.dns),
      _Metric('PX',     '/api/paranoidx/status',     Icons.shield),
      _Metric('Oracle', '/api/economy/oracle',       Icons.trending_up),
      _Metric('Docker', '/api/admin/docker',         Icons.view_in_ar),
    ]);
    WidgetsBinding.instance.addPostFrameCallback((_) => _cycle());
  }

  @override
  void dispose() { super.dispose(); }

  Future<void> _cycle() async {
    while (mounted) {
      setState(() { _ready = false; _polling = true; });
      for (final m in _all) { m.state = _MState.pending; m.text = '—'; }

      final client = http.Client();
      try {
        for (final m in _all) {
          if (!mounted) { client.close(); return; }
          setState(() => m.state = _MState.busy);
          final ok = await _fetch(client, m);
          if (!mounted) { client.close(); return; }
          if (ok) {
            m.fails = 0;
            m.state = _MState.ok;
          } else {
            m.fails++;
            m.state = m.fails >= 3 ? _MState.stuck : _MState.fail;
            if (m.fails >= 3) _revive(m);
          }
        }
      } finally { client.close(); }

      if (!mounted) return;
      setState(() { _ready = true; _polling = false; });
      await Future.delayed(const Duration(seconds: 15));
      if (!mounted) return;
    }
  }

  Future<bool> _fetch(http.Client client, _Metric m) async {
    try {
      final r = await client
          .get(Uri.parse('${widget.apiBase}${m.path}'))
          .timeout(const Duration(seconds: 5));
      if (r.statusCode != 200) return false;
      final d = jsonDecode(r.body) as Map<String, dynamic>;
      return _parse(m, d);
    } catch (_) { return false; }
  }

  bool _parse(_Metric m, Map<String, dynamic> d) {
    switch (m.name) {
      case 'CPU':
        m.value = (d['cpu_percent'] as num?)?.toDouble() ?? 0;
        m.text = '${m.value.toStringAsFixed(1)}%';
        return d.containsKey('cpu_percent');
      case 'RAM':
        final ram = d['ram'] as Map<String, dynamic>? ?? {};
        final pct = (ram['used_pct'] as String?)?.replaceAll('%', '') ?? '0';
        m.value = double.tryParse(pct) ?? 0;
        m.text = '${m.value.toStringAsFixed(1)}%';
        return d.containsKey('ram');
      case 'Disk':
        final disk = d['disk'] as Map<String, dynamic>? ?? {};
        final pct = (disk['used_pct'] as String?)?.replaceAll('%', '') ?? '0';
        m.value = double.tryParse(pct) ?? 0;
        m.text = '${m.value.toStringAsFixed(1)}%';
        return d.containsKey('disk');
      case 'Bridge':
        m.value = d['bridge_connected'] == true || d['connected'] == true;
        m.text = m.value == true ? 'Up' : 'Down';
        return true;
      case 'Node':
        m.text = d['build'] ?? d['version'] ?? '—';
        return true;
      case 'PX':
        final ov = d['overall'] ?? d['status'] ?? '?';
        m.text = '$ov'.toUpperCase();
        return true;
      case 'Oracle':
        m.value = d['price'] ?? d['usd_per_oz'] ?? 0;
        m.text = '\$${m.value}/oz';
        return true;
      case 'Docker':
        final cc = d['containers'] as List? ?? [];
        final up = cc.where((c) => (c is Map && c['state'] == 'running')).length;
        m.text = '$up/${cc.length} up';
        return true;
    }
    return false;
  }

  Future<void> _revive(_Metric m) async {
    m.state = _MState.revive;
    final endpoints = <String, String>{
      'Bridge': '/api/chat/bridge-reconnect',
      'PX':     '/api/paranoidx/chain/rebuild',
    };
    final ep = endpoints[m.name];
    if (ep == null) return;
    try {
      await http.Client()
          .post(Uri.parse('${widget.apiBase}$ep'),
                headers: {'Content-Type': 'application/json'})
          .timeout(const Duration(seconds: 5));
    } catch (_) {}
  }

  String get _statusLine {
    if (!_ready) return 'Collecting telemetry...';
    final ok = _all.where((m) => m.state == _MState.ok).length;
    final bad = _all.where((m) => m.state == _MState.stuck).length;
    final fail = _all.where((m) => m.state == _MState.fail).length;
    if (bad > 0) return '$bad stuck · $fail failed · next cycle 15s';
    if (fail > 0) return '$fail failed · $ok OK · next cycle 15s';
    return '$ok/${_all.length} OK · next cycle 15s';
  }

  @override
  Widget build(BuildContext context) {
    final ok = _all.where((m) => m.state == _MState.ok).length;
    final bad = _all.where((m) => m.state == _MState.stuck).length;
    final headColor = bad > 0 ? const Color(0xFFFF6B6B)
                  : ok == _all.length ? const Color(0xFF3FB950)
                  : const Color(0xFF8B949E);

    return Container(
      margin: const EdgeInsets.only(top: 12),
      padding: const EdgeInsets.all(20),
      decoration: BoxDecoration(
        color: const Color(0xFF161B22),
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: const Color(0xFF30363D)),
      ),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Row(children: [
          const Icon(Icons.monitor_heart, size: 22, color: Color(0xFF58A6FF)),
          const SizedBox(width: 10),
          Text('System Telemetry',
              style: TextStyle(fontSize: 20, fontWeight: FontWeight.w700, color: headColor)),
          const Spacer(),
          if (!_ready)
            const SizedBox(width: 20, height: 20, child: CircularProgressIndicator(strokeWidth: 2)),
          if (bad > 0)
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
              decoration: BoxDecoration(color: const Color(0xFFFF6B6B).withAlpha(30), borderRadius: BorderRadius.circular(8)),
              child: Text('$bad STUCK', style: const TextStyle(fontSize: 12, color: Color(0xFFFF6B6B), fontWeight: FontWeight.w700)),
            ),
        ]),
        const SizedBox(height: 14),
        if (!_ready && !_polling)
          const Padding(
            padding: EdgeInsets.symmetric(vertical: 16),
            child: Center(child: Text('Gathering telemetry...', style: TextStyle(fontSize: 16, color: Color(0xFF8B949E)))),
          )
        else
          ..._all.map((m) => _row(m)),
        if (_ready) ...[
          const SizedBox(height: 8),
          Center(child: Text(_statusLine, style: const TextStyle(fontSize: 13, color: Color(0xFF484F58)))),
        ],
      ]),
    );
  }

  Widget _row(_Metric m) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 5),
      child: Row(children: [
        Icon(m.icon, size: 20, color: m.color),
        const SizedBox(width: 10),
        Text(m.name, style: TextStyle(fontSize: 16, fontWeight: FontWeight.w600, color: m.color)),
        const SizedBox(width: 8),
        if (m.state == _MState.busy)
          const SizedBox(width: 18, height: 18, child: CircularProgressIndicator(strokeWidth: 2))
        else
          Flexible(child: Text(m.text, style: TextStyle(fontSize: 16, color: m.color, fontWeight: FontWeight.w500))),
        const Spacer(),
        Text(m.badge, style: TextStyle(fontSize: 16, color: m.color)),
        if (m.state == _MState.stuck)
          const Padding(padding: EdgeInsets.only(left: 4), child: Icon(Icons.medical_services, size: 16, color: Color(0xFFD29922))),
      ]),
    );
  }
}
