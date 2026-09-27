import 'dart:async';
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../theme.dart';
import '../main.dart';

class SettingsScreen extends StatefulWidget {
  const SettingsScreen({super.key});
  @override
  State<SettingsScreen> createState() => _SettingsScreenState();
}

class _SettingsScreenState extends State<SettingsScreen> {
  Map<String, dynamic>? _info, _emStop, _rateLimit, _containerStatus, _schedule;
  Timer? _timer;

  @override
  void initState() {
    super.initState();
    _refresh();
    _timer = Timer.periodic(const Duration(seconds: 30), (_) => _refresh());
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }

  Future<void> _refresh() async {
    if (!mounted) return;
    final api = context.read<AppState>().api;
    try {
      final r = await Future.wait([
        api.getInfo(),
        api.getEmergencyStop(),
        api.getRateLimitStats(),
        api.getContainerStatus(),
        api.getRadioSchedule(),
      ], eagerError: false);
      if (mounted) {
        setState(() {
          _info = r[0];
          _emStop = r[1];
          _rateLimit = r[2];
          _containerStatus = r[3];
          _schedule = r[4];
        });
      }
    } catch (e) {
      if (mounted) {
        _snack('Refresh failed: $e', RoyalTheme.red);
      }
    }
  }

  Future<void> _toggleEmergencyStop() async {
    final current = _emStop?['emergency_stop'] == true;
    final api = context.read<AppState>().api;
    try {
      await api.setEmergencyStop(!current);
      _refresh();
      if (mounted) {
        _snack(!current ? 'Emergency Stop ENABLED' : 'Emergency Stop DISABLED',
            !current ? RoyalTheme.red : RoyalTheme.green);
      }
    } catch (e) {
      if (mounted) _snack('Error: $e', RoyalTheme.red);
    }
  }

  Future<void> _confirmPanic() async {
    final api = context.read<AppState>().api;
    final confirmed = await showDialog<bool>(
        context: context,
        builder: (ctx) => AlertDialog(
              backgroundColor: RoyalTheme.darkCard,
              title: const Text('⚠ PANIC WIPE',
                  style: TextStyle(color: RoyalTheme.red)),
              content: const Text(
                  'This will DESTROY all containers, keys, and data. The node will be reset to factory state. This cannot be undone!',
                  style: TextStyle(color: Colors.white70)),
              actions: [
                TextButton(
                    onPressed: () => Navigator.pop(ctx, false),
                    child: const Text('Cancel')),
                ElevatedButton(
                  style:
                      ElevatedButton.styleFrom(backgroundColor: RoyalTheme.red),
                  onPressed: () => Navigator.pop(ctx, true),
                  child: const Text('EXECUTE PANIC',
                      style: TextStyle(color: Colors.white)),
                ),
              ],
            ));
    if (confirmed != true) return;
    try {
      await api.panicWipe();
      if (mounted) _snack('PANIC WIPE EXECUTED', RoyalTheme.red);
    } catch (e) {
      if (mounted) _snack('Panic error: $e', RoyalTheme.red);
    }
  }

  Future<void> _restartService(String service) async {
    final api = context.read<AppState>().api;
    try {
      final res = await api.restartService(service);
      if (mounted) {
        _snack(res['ok'] == true ? '$service restarted' : 'Failed',
            res['ok'] == true ? RoyalTheme.green : RoyalTheme.red);
      }
    } catch (e) {
      if (mounted) _snack('Error: $e', RoyalTheme.red);
    }
  }

  Future<void> _triggerBackup() async {
    final api = context.read<AppState>().api;
    try {
      final res = await api.triggerBackup();
      if (mounted) {
        _snack(
            res['ok'] == true ? 'Backup started' : 'Failed', RoyalTheme.green);
      }
    } catch (e) {
      if (mounted) _snack('Error: $e', RoyalTheme.red);
    }
  }

  Future<void> _diskCleanup() async {
    final api = context.read<AppState>().api;
    try {
      final res = await api.diskCleanup();
      if (mounted) {
        _snack(res['ok'] == true ? 'Disk cleanup done' : 'Failed',
            RoyalTheme.green);
      }
    } catch (e) {
      if (mounted) _snack('Error: $e', RoyalTheme.red);
    }
  }

  void _snack(String msg, Color color) {
    ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(msg), backgroundColor: color.withAlpha(40)));
  }

  String _safe(dynamic v, [String def = '—']) => v?.toString() ?? def;

  @override
  Widget build(BuildContext context) {
    final info = _info ?? {};
    final emStop = _emStop?['emergency_stop'] == true;
    final rl = _rateLimit ?? {};
    final container = _containerStatus ?? {};
    final sched = _schedule ?? {};
    return Scaffold(
        appBar: AppBar(
          title: Row(children: [
            Container(
                padding: const EdgeInsets.all(6),
                decoration: BoxDecoration(
                    color: RoyalTheme.silver.withAlpha(30),
                    borderRadius: BorderRadius.circular(8)),
                child: const Icon(Icons.settings,
                    color: RoyalTheme.silver, size: 20)),
            const SizedBox(width: 12),
            const Text('Settings'),
          ]),
          actions: [
            IconButton(icon: const Icon(Icons.refresh), onPressed: _refresh)
          ],
        ),
        body: RefreshIndicator(
          onRefresh: _refresh,
          child: ListView(padding: const EdgeInsets.all(16), children: [
            // Node Info
            Container(
                padding: const EdgeInsets.all(16),
                decoration: RoyalTheme.glowBorder(),
                child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Row(children: [
                        Container(
                            width: 40,
                            height: 40,
                            decoration: BoxDecoration(
                                shape: BoxShape.circle,
                                border: Border.all(
                                    color: RoyalTheme.gold, width: 2)),
                            child: const Center(
                                child: Text('♚',
                                    style: TextStyle(
                                        fontSize: 20,
                                        color: RoyalTheme.gold)))),
                        const SizedBox(width: 12),
                        Text('Royal Node',
                            style: Theme.of(context).textTheme.titleLarge),
                      ]),
                      const SizedBox(height: 12),
                      _kv('Build', _safe(info['build'] ?? info['version'])),
                      _kv('Go', _safe(info['go_version'] ?? info['go'])),
                      _kv('Started', _safe(info['started'])),
                      _kv('Uptime', _safe(info['uptime'])),
                      _kv('Data Dir', _safe(info['data_dir'])),
                      _kv('Goroutines', _safe(info['goroutines'])),
                    ])),
            const SizedBox(height: 16),

            // Service Control
            Container(
                padding: const EdgeInsets.all(16),
                decoration: RoyalTheme.gradientCard(),
                child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Row(children: [
                        const Icon(Icons.settings_backup_restore,
                            color: RoyalTheme.accent, size: 18),
                        const SizedBox(width: 8),
                        Text('Service Control',
                            style: Theme.of(context).textTheme.titleMedium)
                      ]),
                      const SizedBox(height: 12),
                      Wrap(spacing: 8, runSpacing: 8, children: [
                        _serviceBtn('Tor', () => _restartService('tor')),
                        _serviceBtn('SMP', () => _restartService('smp')),
                        _serviceBtn('XFTP', () => _restartService('xftp')),
                        _serviceBtn('Coturn', () => _restartService('coturn')),
                        _serviceBtn('Xray', () => _restartService('xray')),
                      ]),
                    ])),
            const SizedBox(height: 16),

            // Maintenance Actions
            Container(
                padding: const EdgeInsets.all(16),
                decoration: RoyalTheme.gradientCard(),
                child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Row(children: [
                        const Icon(Icons.build,
                            color: RoyalTheme.gold, size: 18),
                        const SizedBox(width: 8),
                        Text('Maintenance',
                            style: Theme.of(context).textTheme.titleMedium)
                      ]),
                      const SizedBox(height: 12),
                      Row(children: [
                        Expanded(
                            child: OutlinedButton.icon(
                                icon: const Icon(Icons.backup, size: 16),
                                label: const Text('Backup'),
                                onPressed: _triggerBackup)),
                        const SizedBox(width: 8),
                        Expanded(
                            child: OutlinedButton.icon(
                                icon: const Icon(Icons.cleaning_services,
                                    size: 16),
                                label: const Text('Disk Cleanup'),
                                onPressed: _diskCleanup)),
                      ]),
                      const SizedBox(height: 8),
                      SizedBox(
                          width: double.infinity,
                          child: OutlinedButton.icon(
                            icon: const Icon(Icons.warning,
                                size: 16, color: RoyalTheme.red),
                            label: const Text('PANIC WIPE',
                                style: TextStyle(color: RoyalTheme.red)),
                            onPressed: _confirmPanic,
                            style: OutlinedButton.styleFrom(
                                side: const BorderSide(color: RoyalTheme.red)),
                          )),
                    ])),
            const SizedBox(height: 16),

            // CryptoContainer
            Container(
                padding: const EdgeInsets.all(16),
                decoration: RoyalTheme.gradientCard(
                    colors: [RoyalTheme.darkCard, const Color(0xFF1E1515)]),
                child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Row(children: [
                        const Icon(Icons.lock,
                            color: RoyalTheme.accent, size: 18),
                        const SizedBox(width: 8),
                        Text('CryptoContainer',
                            style: Theme.of(context).textTheme.titleMedium),
                        const Spacer(),
                        Container(
                          padding: const EdgeInsets.symmetric(
                              horizontal: 8, vertical: 2),
                          decoration: BoxDecoration(
                              color: (container['open'] == true
                                      ? RoyalTheme.green
                                      : RoyalTheme.orange)
                                  .withAlpha(30),
                              borderRadius: BorderRadius.circular(10)),
                          child: Text(
                              container['open'] == true ? 'Open' : 'Closed',
                              style: TextStyle(
                                  fontSize: 13,
                                  color: container['open'] == true
                                      ? RoyalTheme.green
                                      : RoyalTheme.orange)),
                        ),
                      ]),
                      const SizedBox(height: 8),
                      _kv('Exists', container['exists'] == true ? 'Yes' : 'No'),
                      _kv('Files', _safe(container['files'])),
                    ])),
            const SizedBox(height: 16),

            // Radio Schedule
            Container(
                padding: const EdgeInsets.all(16),
                decoration: RoyalTheme.gradientCard(),
                child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Row(children: [
                        const Icon(Icons.radio,
                            color: RoyalTheme.gold, size: 18),
                        const SizedBox(width: 8),
                        Text('Radio Schedule',
                            style: Theme.of(context).textTheme.titleMedium)
                      ]),
                      const SizedBox(height: 8),
                      _kv('Next',
                          _safe(sched['next'] ?? sched['next_schedule'])),
                      _kv('Tracks',
                          _safe(sched['tracks'] ?? sched['track_count'])),
                      _kv('Content', _safe(sched['content_count'])),
                    ])),
            const SizedBox(height: 16),

            // Emergency Stop
            Container(
                padding: const EdgeInsets.all(16),
                decoration: BoxDecoration(
                    color: (emStop ? RoyalTheme.red : RoyalTheme.darkCard)
                        .withAlpha(emStop ? 20 : 0),
                    borderRadius: BorderRadius.circular(16),
                    border: Border.all(
                        color: emStop
                            ? RoyalTheme.red.withAlpha(80)
                            : const Color(0xFF30363D))),
                child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Row(children: [
                        Icon(Icons.warning_amber,
                            color: emStop ? RoyalTheme.red : RoyalTheme.gold,
                            size: 20),
                        const SizedBox(width: 8),
                        Text('Emergency Stop',
                            style: Theme.of(context).textTheme.titleMedium),
                        const Spacer(),
                        Switch(
                            value: emStop,
                            activeThumbColor: RoyalTheme.red,
                            onChanged: (_) => _toggleEmergencyStop()),
                      ]),
                      Text(
                          emStop
                              ? 'ALL treasury operations SUSPENDED'
                              : 'Halt all treasury mint/burn operations',
                          style: TextStyle(
                              fontSize: 13,
                              color: emStop
                                  ? RoyalTheme.red
                                  : const Color(0xFF8B949E))),
                    ])),
            const SizedBox(height: 16),

            // Rate Limits
            Container(
                padding: const EdgeInsets.all(16),
                decoration: RoyalTheme.gradientCard(),
                child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Row(children: [
                        const Icon(Icons.speed,
                            color: RoyalTheme.accent, size: 18),
                        const SizedBox(width: 8),
                        Text('Rate Limits',
                            style: Theme.of(context).textTheme.titleMedium)
                      ]),
                      const SizedBox(height: 8),
                      _kv('Max RPM', _safe(rl['max_rpm'])),
                      _kv('Endpoints', _safe(rl['endpoints'])),
                      _kv('Policy', _safe(rl['policy'])),
                    ])),
          ]),
        ));
  }

  Widget _serviceBtn(String label, VoidCallback onTap) {
    return ActionChip(
        label: Text(label, style: const TextStyle(fontSize: 13)),
        avatar: const Icon(Icons.restart_alt, size: 14),
        backgroundColor: RoyalTheme.darkCard,
        onPressed: onTap);
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
