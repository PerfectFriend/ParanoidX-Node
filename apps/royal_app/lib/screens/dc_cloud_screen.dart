import 'dart:async';
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../theme.dart';
import '../main.dart';

/// DCCloudScreen manages dccloudscreen functionality.
class DCCloudScreen extends StatefulWidget {
  const DCCloudScreen({super.key});
  @override
  State<DCCloudScreen> createState() => _DCCloudScreenState();
}

class _DCCloudScreenState extends State<DCCloudScreen> {
  Map<String, dynamic>? _status, _list, _swarm;
  Timer? _timer;

  @override
  void initState() { super.initState(); _refresh(); _timer = Timer.periodic(const Duration(seconds: 30), (_) => _refresh()); }
  @override
  void dispose() { _timer?.cancel(); super.dispose(); }

  Future<void> _refresh() async {
    final api = context.read<AppState>().api;
    try {
      final r = await Future.wait([api.dcStatus(), api.dcList(), api.dcSwarm()], eagerError: false);
      if (mounted) {
        setState(() {
          _status = r[0];
          _list = r[1];
          _swarm = r[2];
        });
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Refresh failed: $e'), backgroundColor: RoyalTheme.red.withAlpha(40)));
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final st = _status ?? {};
    final containers = (_list?['containers'] as List?) ?? [];
    final swarm = (_swarm?['swarm'] as List?) ?? [];
    final seeding = st['seeding'] ?? st['active_seeds'] ?? 0;
    return Scaffold(
      appBar: AppBar(
        title: Row(children: [
          Container(padding: const EdgeInsets.all(6), decoration: BoxDecoration(color: RoyalTheme.teal.withAlpha(30), borderRadius: BorderRadius.circular(8)),
            child: const Icon(Icons.cloud, color: RoyalTheme.teal, size: 20)),
          const SizedBox(width: 12),
          const Text('DC Cloud'),
        ]),
        actions: [IconButton(icon: const Icon(Icons.refresh), onPressed: _refresh)],
      ),
      body: RefreshIndicator(
        onRefresh: _refresh,
        child: ListView(
          padding: const EdgeInsets.all(16),
          children: [
            Container(
              padding: const EdgeInsets.all(20),
              decoration: RoyalTheme.glowBorder(color: RoyalTheme.teal),
              child: Row(mainAxisAlignment: MainAxisAlignment.spaceAround, children: [
                _bigStat('Seeding', '$seeding', RoyalTheme.green),
                _bigStat('Containers', '${containers.length}', RoyalTheme.accent),
                _bigStat('Peers', '${swarm.length}', RoyalTheme.purple),
              ]),
            ),
            const SizedBox(height: 16),
            Text('Container Registry', style: Theme.of(context).textTheme.titleMedium),
            const SizedBox(height: 12),
            if (containers.isEmpty)
              Container(
                padding: const EdgeInsets.all(32),
                decoration: RoyalTheme.gradientCard(),
                child: Center(child: Text('No containers registered', style: Theme.of(context).textTheme.bodyMedium)),
              )
            else
              ...containers.map((c) => _containerCard(c as Map<String, dynamic>)),
          ],
        ),
      ),
    );
  }

  Widget _containerCard(Map<String, dynamic> c) {
    final name = c['name'] ?? c['id'] ?? 'Unknown';
    final hash = c['infohash'] ?? c['hash'] ?? '';
    final size = c['size'] ?? c['total_size'] ?? 0;
    return Container(
      margin: const EdgeInsets.only(bottom: 8),
      padding: const EdgeInsets.all(14),
      decoration: RoyalTheme.gradientCard(),
      child: Row(children: [
        const Icon(Icons.inventory_2, color: RoyalTheme.teal, size: 20),
        const SizedBox(width: 12),
        Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Text('$name', style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 14)),
          Text('${hash.toString().length > 16 ? hash.toString().substring(0, 16) : hash}... | $size B',
            style: Theme.of(context).textTheme.bodyMedium),
        ])),
        const Icon(Icons.check_circle, color: RoyalTheme.green, size: 18),
      ]),
    );
  }

  Widget _bigStat(String label, String value, Color color) {
    return Column(children: [
      Text(value, style: TextStyle(fontSize: 28, fontWeight: FontWeight.w700, color: color)),
      Text(label, style: Theme.of(context).textTheme.bodyMedium),
    ]);
  }
}
