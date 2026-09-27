import 'package:flutter/material.dart';
import 'package:api_client/api_client.dart' show SimplexApiClient;
import 'package:models/models.dart' show Identity, TreasuryState, NodeInfo, SystemMetrics, DockerStatus, ServiceStatus, Diagnostics, OraclePrice, DeflationState, AutoMintConfig, DividendPool, DividendHistory, Banknote, ContainerStatus;

/// RoyalScreen - Admin interface for treasury, system, economy, dividends, and banknotes
class RoyalScreen extends StatefulWidget {
  final SimplexApiClient client;
  final Identity identity;

  const RoyalScreen({super.key, required this.client, required this.identity});

  @override
  State<RoyalScreen> createState() => _RoyalScreenState();
}

class _RoyalScreenState extends State<RoyalScreen> with SingleTickerProviderStateMixin {
  late TabController _tabs;
  TreasuryState? _treasury;
  NodeInfo? _nodeInfo;
  SystemMetrics? _systemMetrics;
  DockerStatus? _docker;
  ServiceStatus? _services;
  Diagnostics? _diagnostics;
  OraclePrice? _oracle;
  DeflationState? _deflation;
  AutoMintConfig? _autoMint;
  DividendPool? _dividendPool;
  List<DividendHistory> _dividendHistory = [];
  List<Banknote> _banknotes = [];
  bool _loading = true;
  String? _error;

  @override
  void initState() {
    super.initState();
    _tabs = TabController(length: 5, vsync: this);
    _load();
  }

  @override
  void dispose() {
    _tabs.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    setState(() => _loading = true);
    try {
      final results = await Future.wait([
        widget.client.royal.treasury.state(),
        widget.client.royal.system.info(),
        widget.client.royal.system.metrics(),
        widget.client.royal.system.docker(),
        widget.client.royal.system.services(),
        widget.client.royal.system.diagnostics(),
        widget.client.royal.treasury.oracle(),
        widget.client.royal.treasury.deflation(),
        widget.client.royal.treasury.autoMintConfig(),
        widget.client.royal.treasury.dividendPool(),
        widget.client.royal.treasury.dividendHistory(limit: 20),
        widget.client.royal.treasury.banknotes(),
      ]);
      if (mounted) {
        setState(() {
          _treasury = results[0] as TreasuryState;
          _nodeInfo = results[1] as NodeInfo;
          _systemMetrics = results[2] as SystemMetrics;
          _docker = results[3] as DockerStatus;
          _services = results[4] as ServiceStatus;
          _diagnostics = results[5] as Diagnostics;
          _oracle = results[6] as OraclePrice;
          _deflation = results[7] as DeflationState;
          _autoMint = results[8] as AutoMintConfig;
          _dividendPool = results[9] as DividendPool;
          _dividendHistory = (results[10] as List).cast<DividendHistory>();
          _banknotes = (results[11] as List).cast<Banknote>();
          _loading = false;
          _error = null;
        });
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _loading = false;
          _error = e.toString();
        });
      }
    }
  }

  String _formatNt(int ng) => (ng / 1000000000).toStringAsFixed(9);

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    if (_loading) {
      return const Scaffold(body: Center(child: CircularProgressIndicator()));
    }

    if (_error != null) {
      return Scaffold(
        appBar: AppBar(title: const Text('Royal Admin'), actions: [IconButton(icon: const Icon(Icons.refresh), onPressed: _load)]),
        body: Center(
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              const Icon(Icons.error_outline, size: 48, color: Colors.red),
              const SizedBox(height: 16),
              Text('Error: $_error', style: const TextStyle(color: Colors.red)),
              const SizedBox(height: 16),
              FilledButton(onPressed: _load, child: const Text('Retry')),
            ],
          ),
        ),
      );
    }

    final t = _treasury!;

    return Scaffold(
      appBar: AppBar(
        title: const Text('Royal Admin'),
        actions: [IconButton(icon: const Icon(Icons.refresh), onPressed: _load)],
        bottom: TabBar(
          controller: _tabs,
          isScrollable: true,
          tabs: const [
            Tab(text: 'Treasury'),
            Tab(text: 'System'),
            Tab(text: 'Economy'),
            Tab(text: 'Dividends'),
            Tab(text: 'Banknotes'),
          ],
        ),
      ),
      body: TabBarView(
        controller: _tabs,
        children: [
          _buildTreasuryTab(theme, t),
          _buildSystemTab(theme),
          _buildEconomyTab(theme),
          _buildDividendsTab(theme),
          _buildBanknotesTab(theme),
        ],
      ),
    );
  }

  Widget _buildTreasuryTab(ThemeData theme, TreasuryState t) {
    return RefreshIndicator(
      onRefresh: _load,
      child: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          _SectionCard(
            title: 'Treasury State',
            children: [
              _StatRow(label: 'Reserve (ng)', value: t.reserveNg.toString(), icon: Icons.account_balance, color: Colors.purple),
              _StatRow(label: 'Supply (ng)', value: t.supplyNg.toString(), icon: Icons.monetization_on, color: Colors.green),
              _StatRow(label: 'Backing Ratio', value: t.backingRatio != null ? '${(t.backingRatio! * 100).toStringAsFixed(1)}%' : 'N/A', icon: Icons.shield, color: Colors.blue),
              _StatRow(label: 'Utility Premium', value: 'N/A', icon: Icons.star, color: Colors.amber),
              _StatRow(label: 'Total Dividends', value: _dividendPool != null ? '${_formatNt(_dividendPool!.distributedNg)} NT' : 'N/A', icon: Icons.paid, color: Colors.amber),
              _StatRow(label: 'Total Minted', value: 'N/A', icon: Icons.add_circle, color: Colors.green),
              _StatRow(label: 'Total Burned', value: 'N/A', icon: Icons.remove_circle, color: Colors.red),
            ],
          ),
          const SizedBox(height: 16),
          _SectionCard(
            title: 'Deflation',
            children: [
              _StatRow(label: 'Deflation Interval', value: '${((t.deflationIntervalMs ?? 0) ~/ 60000)} min', icon: Icons.schedule, color: Colors.blue),
              _StatRow(label: 'Deflation Rate', value: '${((t.deflationRate ?? 0) * 100).toStringAsFixed(2)}%', icon: Icons.local_fire_department, color: Colors.red),
              _StatRow(label: 'Last Dividend', value: t.lastDividendAt != null && t.lastDividendAt! > 0 ? DateTime.fromMillisecondsSinceEpoch(t.lastDividendAt!).toString() : 'Never', icon: Icons.history, color: Colors.grey),
            ],
          ),
          const SizedBox(height: 16),
          _SectionCard(
            title: 'Reserves',
            children: [
              _StatRow(label: 'Silver (oz)', value: (t.silverReserveOz ?? 0).toStringAsFixed(2), icon: Icons.diamond, color: Colors.grey.shade400),
            ],
          ),
        ],
      ),
    );
  }

  Widget _buildSystemTab(ThemeData theme) {
    final s = _nodeInfo!;
    final sys = _systemMetrics!;
    final d = _docker!;
    final svc = _services!;
    final diag = _diagnostics!;
    return RefreshIndicator(
      onRefresh: _load,
      child: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          _SectionCard(
            title: 'System Status',
            children: [
              _StatRow(label: 'Status', value: s.status, icon: Icons.info, color: theme.colorScheme.primary),
              _StatRow(label: 'Uptime', value: s.uptimeFormatted, icon: Icons.timer, color: Colors.blue),
              _StatRow(label: 'Total Banknotes', value: s.totalBanknotes.toString(), icon: Icons.account_balance_wallet, color: Colors.amber),
              _StatRow(label: 'Total Holders', value: s.totalHolders.toString(), icon: Icons.people, color: Colors.blue),
              _StatRow(label: 'Reserve (ng)', value: s.reserveNg.toString(), icon: Icons.account_balance, color: Colors.purple),
              _StatRow(label: 'Supply (ng)', value: s.supplyNg.toString(), icon: Icons.monetization_on, color: Colors.green),
              _StatRow(label: 'Active Users', value: s.activeUsers.toString(), icon: Icons.person, color: Colors.cyan),
            ],
          ),
          const SizedBox(height: 16),
          _SectionCard(
            title: 'Health & Services',
            children: [
              _StatRow(label: 'Docker', value: d.running ? 'Running' : 'Stopped', icon: Icons.memory, color: d.running ? Colors.green : Colors.red),
              _StatRow(label: 'Services', value: svc.allOk ? 'All OK' : 'Issues', icon: Icons.health_and_safety, color: svc.allOk ? Colors.green : Colors.orange),
              _StatRow(label: 'Disk Usage', value: '${diag.diskUsagePercent}%', icon: Icons.storage, color: diag.diskUsagePercent > 80 ? Colors.red : Colors.green),
              _StatRow(label: 'Disk Available', value: '${diag.diskAvailableMb} MB', icon: Icons.sd_storage, color: Colors.blue),
              _StatRow(label: 'Memory', value: '${diag.memoryUsagePercent}%', icon: Icons.memory, color: Colors.purple),
              _StatRow(label: 'CPU', value: '${diag.cpuUsagePercent}%', icon: Icons.speed, color: Colors.orange),
            ],
          ),
          const SizedBox(height: 16),
          Card(
            child: Padding(
              padding: const EdgeInsets.all(16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text('System Actions', style: theme.textTheme.titleMedium?.copyWith(fontWeight: FontWeight.bold)),
                  const SizedBox(height: 12),
                  Wrap(
                    spacing: 12,
                    runSpacing: 8,
                    children: [
                      FilledButton.icon(onPressed: () => _restartService('tor'), icon: const Icon(Icons.lock), label: const Text('Restart Tor')),
                      FilledButton.icon(onPressed: () => _restartService('bridge'), icon: const Icon(Icons.link), label: const Text('Restart Bridge')),
                      FilledButton.icon(onPressed: () => _restartService('simplex-node'), icon: const Icon(Icons.memory), label: const Text('Restart Node')),
                      OutlinedButton.icon(onPressed: _backup, icon: const Icon(Icons.backup), label: const Text('Backup Now'), style: OutlinedButton.styleFrom(foregroundColor: Colors.blue)),
                      OutlinedButton.icon(onPressed: _diskCleanup, icon: const Icon(Icons.cleaning_services), label: const Text('Disk Cleanup'), style: OutlinedButton.styleFrom(foregroundColor: Colors.orange)),
                    ],
                  ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildEconomyTab(ThemeData theme) {
    final o = _oracle!;
    final d = _deflation!;
    final a = _autoMint!;
    return RefreshIndicator(
      onRefresh: _load,
      child: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          _SectionCard(
            title: 'Oracle Price',
            children: [
              _StatRow(label: 'Silver Spot (USD/oz)', value: '\$${o.price.toStringAsFixed(2)}', icon: Icons.attach_money, color: Colors.green),
              _StatRow(label: '24h Change', value: '${o.change24h.toStringAsFixed(2)}%', icon: Icons.trending_up, color: o.change24h >= 0 ? Colors.green : Colors.red),
            ],
          ),
          const SizedBox(height: 16),
          _SectionCard(
            title: 'Deflation',
            children: [
              _StatRow(label: 'Enabled', value: d.enabled ? 'Yes' : 'No', icon: d.enabled ? Icons.check_circle : Icons.cancel, color: d.enabled ? Colors.green : Colors.red),
              _StatRow(label: 'Interval', value: '${d.intervalMs ~/ 60000} min', icon: Icons.schedule, color: Colors.blue),
              _StatRow(label: 'Rate', value: '${(d.rate * 100).toStringAsFixed(2)}%', icon: Icons.local_fire_department, color: Colors.red),
            ],
          ),
          const SizedBox(height: 16),
          _SectionCard(
            title: 'Auto-Mint',
            children: [
              _StatRow(label: 'Enabled', value: a.enabled ? 'Yes' : 'No', icon: a.enabled ? Icons.check_circle : Icons.cancel, color: a.enabled ? Colors.green : Colors.red),
              _StatRow(label: 'Interval', value: '${a.intervalMs ~/ 60000} min', icon: Icons.schedule, color: Colors.blue),
            ],
          ),
          const SizedBox(height: 16),
          Card(
            child: Padding(
              padding: const EdgeInsets.all(16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text('Economy Actions', style: theme.textTheme.titleMedium?.copyWith(fontWeight: FontWeight.bold)),
                  const SizedBox(height: 12),
                  Row(
                    children: [
                      Expanded(child: FilledButton.icon(onPressed: _updateOracle, icon: const Icon(Icons.update), label: const Text('Update Oracle'))),
                      const SizedBox(width: 12),
                      Expanded(child: OutlinedButton.icon(onPressed: _triggerDeflation, icon: const Icon(Icons.local_fire_department), label: const Text('Trigger Deflation'), style: OutlinedButton.styleFrom(foregroundColor: Colors.red))),
                    ],
                  ),
                  const SizedBox(height: 8),
                  Row(
                    children: [
                      Expanded(child: FilledButton.icon(onPressed: () => _updateAutoMint(!a.enabled), icon: Icon(a.enabled ? Icons.pause : Icons.play_arrow), label: Text(a.enabled ? 'Disable Auto-Mint' : 'Enable Auto-Mint'))),
                    ],
                  ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }

  Future<void> _updateOracle() async {
    final ctrl = TextEditingController(text: _oracle!.price.toString());
    final result = await showDialog<double>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Update Oracle Price'),
        content: TextField(controller: ctrl, decoration: const InputDecoration(labelText: 'USD/oz', border: OutlineInputBorder()), keyboardType: TextInputType.number),
        actions: [TextButton(onPressed: () => Navigator.pop(ctx), child: const Text('Cancel')), FilledButton(onPressed: () => Navigator.pop(ctx, double.tryParse(ctrl.text)), child: const Text('Update'))],
      ),
    );
    if (result == null) return;
    try {
      await widget.client.royal.treasury.updateOracle(result);
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Oracle updated')));
      await _load();
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Failed: $e')));
    }
  }

  Future<void> _triggerDeflation() async {
    try {
      await widget.client.royal.treasury.triggerDeflation();
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Deflation triggered')));
      await _load();
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Failed: $e')));
    }
  }

  Future<void> _updateAutoMint(bool enabled) async {
    try {
      await widget.client.royal.treasury.updateAutoMint(enabled, _autoMint!.intervalMs);
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Auto-mint ${enabled ? 'enabled' : 'disabled'}')));
      await _load();
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Failed: $e')));
    }
  }

  Future<void> _restartService(String service) async {
    try {
      await widget.client.royal.system.restartService(service);
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('$service restarted')));
      await _load();
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Failed: $e')));
    }
  }

  Future<void> _backup() async {
    try {
      await widget.client.royal.system.backup();
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Backup initiated')));
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Failed: $e')));
    }
  }

  Future<void> _diskCleanup() async {
    try {
      await widget.client.royal.system.diskCleanup();
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Disk cleanup complete')));
      await _load();
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Failed: $e')));
    }
  }

  Widget _buildDividendsTab(ThemeData theme) {
    final dp = _dividendPool!;
    return RefreshIndicator(
      onRefresh: _load,
      child: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          _SectionCard(
            title: 'Dividend Pool',
            children: [
              _StatRow(label: 'Balance', value: '${_formatNt(dp.balanceNg)} NT', icon: Icons.account_balance, color: Colors.amber),
              _StatRow(label: 'Distributed', value: '${_formatNt(dp.distributedNg)} NT', icon: Icons.paid, color: Colors.green),
              _StatRow(label: 'Last Distribution', value: dp.lastDistribution.isNotEmpty ? dp.lastDistribution : 'Never', icon: Icons.history, color: Colors.blue),
            ],
          ),
          const SizedBox(height: 16),
          Card(
            child: Padding(
              padding: const EdgeInsets.all(16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text('Actions', style: theme.textTheme.titleMedium?.copyWith(fontWeight: FontWeight.bold)),
                  const SizedBox(height: 12),
                  Row(
                    children: [
                      Expanded(child: FilledButton.icon(onPressed: _triggerDividend, icon: const Icon(Icons.payment), label: const Text('Trigger Dividend'))),
                      const SizedBox(width: 12),
                      Expanded(child: OutlinedButton.icon(onPressed: () => _triggerDividendWithPool(1000000000), icon: const Icon(Icons.attach_money), label: const Text('Trigger (1 NT)'), style: OutlinedButton.styleFrom(foregroundColor: Colors.green))),
                    ],
                  ),
                ],
              ),
            ),
          ),
          const SizedBox(height: 16),
          _SectionCard(
            title: 'Dividend History (${_dividendHistory.length})',
            children: _dividendHistory.isEmpty
                ? [const Center(child: Padding(padding: EdgeInsets.all(16), child: Text('No dividend history')))]
                : _dividendHistory.map((h) => Card(
                      margin: const EdgeInsets.symmetric(vertical: 4),
                      child: ListTile(
                        leading: Icon(Icons.attach_money, color: Colors.amber.shade700),
                        title: Text('${_formatNt(h.amountNg)} NT'),
                        subtitle: Text('${h.recipients} recipients • ${h.timestamp}'),
                        trailing: Text('${(h.amountNg / h.recipients / 1000000000).toStringAsFixed(9)} NT each', style: TextStyle(fontSize: 11, color: Colors.grey[600])),
                      ),
                    )).toList(),
          ),
        ],
      ),
    );
  }

  Future<void> _triggerDividend() async {
    try {
      await widget.client.royal.treasury.triggerDividend();
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Dividend triggered')));
      await _load();
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Failed: $e')));
    }
  }

  Future<void> _triggerDividendWithPool(int poolNg) async {
    try {
      await widget.client.royal.treasury.triggerDividend(poolNg: poolNg);
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Dividend triggered with custom pool')));
      await _load();
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Failed: $e')));
    }
  }

  Widget _buildBanknotesTab(ThemeData theme) {
    return RefreshIndicator(
      onRefresh: _load,
      child: _banknotes.isEmpty
          ? const Center(child: Text('No banknotes'))
          : ListView.builder(
              padding: const EdgeInsets.all(12),
              itemCount: _banknotes.length,
              itemBuilder: (ctx, i) {
                final b = _banknotes[i];
                return Card(
                  margin: const EdgeInsets.symmetric(vertical: 6),
                  child: ListTile(
                    leading: Icon(Icons.monetization_on, color: Colors.amber.shade700),
                    title: Text(b.serial, style: const TextStyle(fontFamily: 'monospace')),
                    subtitle: Text('${_formatNt(b.denominationNg)} NT • ${b.rarityLabel} • ${b.status}'),
                    trailing: Text(b.holder.substring(0, 12) + '...', style: TextStyle(fontSize: 11, color: Colors.grey[600])),
                  ),
                );
              },
            ),
    );
  }
}

class _SectionCard extends StatelessWidget {
  final String title;
  final List<Widget> children;

  const _SectionCard({required this.title, required this.children});

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(title, style: theme.textTheme.titleMedium?.copyWith(fontWeight: FontWeight.bold)),
            const SizedBox(height: 12),
            ...children,
          ],
        ),
      ),
    );
  }
}

class _StatRow extends StatelessWidget {
  final String label;
  final String value;
  final IconData icon;
  final Color color;

  const _StatRow({required this.label, required this.value, required this.icon, required this.color});

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 6),
      child: Row(
        children: [
          Icon(icon, color: color, size: 20),
          const SizedBox(width: 12),
          Expanded(child: Text(label, style: theme.textTheme.bodyMedium)),
          Text(value, style: theme.textTheme.bodyMedium?.copyWith(fontWeight: FontWeight.w500)),
        ],
      ),
    );
  }
}