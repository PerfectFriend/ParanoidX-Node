import 'dart:async';
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../theme.dart';
import '../main.dart';

class TreasuryScreen extends StatefulWidget {
  const TreasuryScreen({super.key});
  @override
  State<TreasuryScreen> createState() => _TreasuryScreenState();
}

class _TreasuryScreenState extends State<TreasuryScreen> {
  Map<String, dynamic>? _reserve,
      _oracle,
      _tokenomics,
      _forecast,
      _audit,
      _silverAssets;
  Timer? _timer;
  bool _isLoading = true;
  String? _error;

  final _mintCtrl = TextEditingController();
  final _burnCtrl = TextEditingController();
  final _priceCtrl = TextEditingController();
  final _dividendCtrl = TextEditingController();

  @override
  void initState() {
    super.initState();
    _refresh();
    _timer = Timer.periodic(const Duration(seconds: 30), (_) => _refresh());
  }

  @override
  void dispose() {
    _timer?.cancel();
    _mintCtrl.dispose();
    _burnCtrl.dispose();
    _priceCtrl.dispose();
    _dividendCtrl.dispose();
    super.dispose();
  }

  Future<void> _refresh() async {
    final api = context.read<AppState>().api;
    try {
      final r = await Future.wait([
        api.getReserve(),
        api.getSilverOracle(),
        api.getTokenomics(),
        api.getForecast(),
        api.getAuditLog(),
        api.getSilverAssets(),
      ], eagerError: false);
      if (mounted) {
        setState(() {
          _reserve = r[0];
          _oracle = r[1];
          _tokenomics = r[2];
          _forecast = r[3];
          _audit = r[4];
          _silverAssets = r[5];
          _isLoading = false;
          _error = null;
        });
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _isLoading = false;
          _error = '$e';
        });
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(
            content: Text('Refresh failed: $e'),
            backgroundColor: RoyalTheme.red.withAlpha(40)));
      }
    }
  }

  Future<void> _action(
      String label, Future<Map<String, dynamic>> Function() fn) async {
    try {
      final res = await fn();
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(
          content: Text('${res['ok'] == true ? '✓' : '✗'} $label'),
          backgroundColor: res['ok'] == true
              ? RoyalTheme.green.withAlpha(40)
              : RoyalTheme.red.withAlpha(40),
        ));
        _refresh();
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(
            content: Text('Error: $e'),
            backgroundColor: RoyalTheme.red.withAlpha(40)));
      }
    }
  }

  void _showMintDialog() {
    showDialog(
        context: context,
        builder: (ctx) => AlertDialog(
              backgroundColor: RoyalTheme.darkCard,
              title: const Text('Mint Silver Asset'),
              content: Column(mainAxisSize: MainAxisSize.min, children: [
                TextField(
                  controller: _mintCtrl,
                  keyboardType: TextInputType.number,
                  decoration: const InputDecoration(labelText: 'Amount (ng)'),
                ),
                const SizedBox(height: 8),
                Text('Creates a silver-backed asset in treasury',
                    style: TextStyle(
                        fontSize: 13, color: RoyalTheme.gold.withAlpha(180))),
              ]),
              actions: [
                TextButton(
                    onPressed: () => Navigator.pop(ctx),
                    child: const Text('Cancel')),
                ElevatedButton(
                    onPressed: () {
                      final amt = int.tryParse(_mintCtrl.text);
                      if (amt != null && amt > 0) {
                        _action('Mint $amt ng',
                            () => context.read<AppState>().api.mint(amt));
                        Navigator.pop(ctx);
                      }
                    },
                    child: const Text('Mint')),
              ],
            ));
  }

  void _showBurnDialog() {
    final assets = (_silverAssets?['assets'] as List?) ?? [];
    if (assets.isEmpty) {
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(
        content: const Text('No silver assets to burn'),
        backgroundColor: RoyalTheme.orange.withAlpha(40),
      ));
      return;
    }
    String? selectedId;
    showDialog(
        context: context,
        builder: (ctx) => AlertDialog(
              backgroundColor: RoyalTheme.darkCard,
              title: const Text('Burn Silver Asset'),
              content: Column(mainAxisSize: MainAxisSize.min, children: [
                DropdownButtonFormField<String>(
                  dropdownColor: RoyalTheme.darkCard,
                  decoration: const InputDecoration(labelText: 'Asset'),
                  items: assets.map((a) {
                    final m = a as Map<String, dynamic>;
                    final id = m['id']?.toString() ?? '';
                    return DropdownMenuItem<String>(
                        value: id,
                        child: Text(
                            '${id.length > 20 ? id.substring(0, 20) : id}… (${m['amount_ng']} ng)'));
                  }).toList(),
                  onChanged: (v) => selectedId = v,
                ),
              ]),
              actions: [
                TextButton(
                    onPressed: () => Navigator.pop(ctx),
                    child: const Text('Cancel')),
                ElevatedButton(
                  style:
                      ElevatedButton.styleFrom(backgroundColor: RoyalTheme.red),
                  onPressed: selectedId != null
                      ? () {
                          _action(
                              'Burn',
                              () => context
                                  .read<AppState>()
                                  .api
                                  .burn(selectedId!));
                          Navigator.pop(ctx);
                        }
                      : null,
                  child: const Text('Burn'),
                ),
              ],
            ));
  }

  void _showOracleDialog() {
    showDialog(
        context: context,
        builder: (ctx) => AlertDialog(
              backgroundColor: RoyalTheme.darkCard,
              title: const Text('Set Oracle Price'),
              content: TextField(
                controller: _priceCtrl,
                keyboardType: TextInputType.number,
                decoration: const InputDecoration(labelText: 'Price (USD/oz)'),
              ),
              actions: [
                TextButton(
                    onPressed: () => Navigator.pop(ctx),
                    child: const Text('Cancel')),
                ElevatedButton(
                    onPressed: () {
                      final p = double.tryParse(_priceCtrl.text);
                      if (p != null && p > 0) {
                        _action('Update Oracle',
                            () => context.read<AppState>().api.updateOracle(p));
                        Navigator.pop(ctx);
                      }
                    },
                    child: const Text('Update')),
              ],
            ));
  }

  void _showDividendDialog() {
    showDialog(
        context: context,
        builder: (ctx) => AlertDialog(
              backgroundColor: RoyalTheme.darkCard,
              title: const Text('Trigger Dividend'),
              content: TextField(
                controller: _dividendCtrl,
                keyboardType: TextInputType.number,
                decoration: const InputDecoration(
                    labelText: 'Pool (ng)', hintText: '0 = auto'),
              ),
              actions: [
                TextButton(
                    onPressed: () => Navigator.pop(ctx),
                    child: const Text('Cancel')),
                ElevatedButton(
                    onPressed: () {
                      final pool = int.tryParse(_dividendCtrl.text) ?? 0;
                      _action(
                          'Dividend',
                          () => context
                              .read<AppState>()
                              .api
                              .triggerDividend(poolNg: pool));
                      Navigator.pop(ctx);
                    },
                    child: const Text('Trigger')),
              ],
            ));
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: Row(children: [
          Container(
              padding: const EdgeInsets.all(6),
              decoration: BoxDecoration(
                  color: RoyalTheme.gold.withAlpha(30),
                  borderRadius: BorderRadius.circular(8)),
              child: const Icon(Icons.account_balance,
                  color: RoyalTheme.gold, size: 20)),
          const SizedBox(width: 12),
          const Text('Treasury'),
        ]),
        actions: [
          IconButton(icon: const Icon(Icons.refresh), onPressed: _refresh)
        ],
      ),
      body: _isLoading
          ? const Center(child: CircularProgressIndicator(color: RoyalTheme.gold))
          : _error != null && _reserve == null
              ? _buildErrorView()
              : RefreshIndicator(
                  onRefresh: _refresh,
                  child: ListView(
                    padding: const EdgeInsets.all(16),
                    children: [
                      _buildOverviewRow(),
                      const SizedBox(height: 16),
                      _buildActionGrid(),
                      const SizedBox(height: 16),
                      _buildTokenomicsCard(),
                      const SizedBox(height: 16),
                      _buildForecastCard(),
                      const SizedBox(height: 16),
                      _buildAuditCard(),
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
            Text('Cannot load treasury data',
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

  Widget _buildOverviewRow() {
    final r = _reserve ?? {};
    final o = _oracle ?? {};
    final reserve = r['reserve_ng'] ?? 0;
    final supply = r['supply_ng'] ?? 0;
    final price = o['current_price'] ?? r['silver_spot_usd'] ?? 0;
    final tier = r['tier'] ?? 'Tier0';
    return Container(
      padding: const EdgeInsets.all(20),
      decoration: RoyalTheme.glowBorder(),
      child: Column(children: [
        Row(mainAxisAlignment: MainAxisAlignment.spaceAround, children: [
          _overviewItem('Reserve', '$reserve ng', RoyalTheme.gold),
          _overviewItem('Supply', '$supply ng', RoyalTheme.silver),
          _overviewItem('Price', '\$$price', RoyalTheme.green),
          _overviewItem('Tier', '$tier', Colors.blue),
        ]),
      ]),
    );
  }

  Widget _overviewItem(String label, String value, Color? color) {
    final c = color ?? RoyalTheme.gold;
    return Column(children: [
      Text(value,
          style:
              TextStyle(fontSize: 18, fontWeight: FontWeight.w700, color: c)),
      Text(label, style: Theme.of(context).textTheme.bodyMedium),
    ]);
  }

  Widget _buildActionGrid() {
    return Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
      Text('Treasury Actions', style: Theme.of(context).textTheme.titleMedium),
      const SizedBox(height: 12),
      Wrap(spacing: 10, runSpacing: 10, children: [
        _actionButton(
            'Mint', Icons.add_circle, RoyalTheme.green, _showMintDialog),
        _actionButton(
            'Burn', Icons.remove_circle, RoyalTheme.red, _showBurnDialog),
        _actionButton(
            'Oracle', Icons.trending_up, RoyalTheme.accent, _showOracleDialog),
        _actionButton(
            'Deflation',
            Icons.compress,
            RoyalTheme.orange,
            () => _action('Deflation',
                () => context.read<AppState>().api.triggerDeflation())),
        _actionButton(
            'Dividend', Icons.payments, RoyalTheme.gold, _showDividendDialog),
      ]),
    ]);
  }

  Widget _actionButton(
      String label, IconData icon, Color color, VoidCallback onTap) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        width: (MediaQuery.of(context).size.width - 52) / 3,
        padding: const EdgeInsets.symmetric(vertical: 16),
        decoration: BoxDecoration(
          color: color.withAlpha(15),
          borderRadius: BorderRadius.circular(12),
          border: Border.all(color: color.withAlpha(50)),
        ),
        child: Column(children: [
          Icon(icon, color: color, size: 24),
          const SizedBox(height: 6),
          Text(label,
              style: TextStyle(
                  fontSize: 13, fontWeight: FontWeight.w600, color: color)),
        ]),
      ),
    );
  }

  Widget _buildTokenomicsCard() {
    final t = _tokenomics ?? {};
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: RoyalTheme.gradientCard(),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Text('Tokenomics', style: Theme.of(context).textTheme.titleMedium),
        const SizedBox(height: 12),
        _kvRow('NG per TLR', '${t['ng_per_tlr'] ?? '—'}'),
        _kvRow('Silver Spot', '\$${t['silver_spot_usd_per_oz'] ?? '—'}'),
        _kvRow('Backing Ratio', '${t['silver_backing_ratio'] ?? '—'}'),
        _kvRow('Commission', '${t['treasury_commission_bps'] ?? 0} bps'),
        _kvRow('Dividend Share', '${t['dividend_share_bps'] ?? 0} bps'),
        _kvRow('Treasury Ops', '${t['treasury_monthly_ops_ng'] ?? '—'} ng/mo'),
      ]),
    );
  }

  Widget _buildForecastCard() {
    final f = _forecast ?? {};
    final health = f['health'] ?? 'unknown';
    final healthColor = health == 'healthy'
        ? RoyalTheme.green
        : (health == 'warn' ? RoyalTheme.orange : RoyalTheme.red);
    final recs = f['recommendations'] as List? ?? [];
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: RoyalTheme.gradientCard(
          colors: [RoyalTheme.darkCard, const Color(0xFF111820)]),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Row(children: [
          const Icon(Icons.trending_up, color: RoyalTheme.teal, size: 18),
          const SizedBox(width: 8),
          Text('Forecast', style: Theme.of(context).textTheme.titleMedium),
          const Spacer(),
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
            decoration: BoxDecoration(
                color: healthColor.withAlpha(30),
                borderRadius: BorderRadius.circular(10)),
            child: Text('$health',
                style: TextStyle(fontSize: 14, color: healthColor)),
          ),
        ]),
        const SizedBox(height: 12),
        _kvRow('Reserve', '${f['reserve_ng'] ?? '—'} ng'),
        _kvRow('Supply', '${f['supply_ng'] ?? '—'} ng'),
        _kvRow('Tier', '${f['tier'] ?? '—'}'),
        _kvRow('Months until depleted', '${f['months_until_depleted'] ?? '—'}'),
        _kvRow('Total USD Value', '\$${f['total_usd_value'] ?? '—'}'),
        if (recs.isNotEmpty) ...[
          const SizedBox(height: 8),
          const Text('Recommendations:',
              style: TextStyle(fontSize: 13, color: RoyalTheme.gold)),
          ...recs.map((r) => Padding(
                padding: const EdgeInsets.only(top: 4),
                child: Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      const Text('• ',
                          style:
                              TextStyle(color: RoyalTheme.gold, fontSize: 13)),
                      Expanded(
                          child: Text('$r',
                              style: TextStyle(
                                  fontSize: 13,
                                  color: RoyalTheme.gold.withAlpha(180)))),
                    ]),
              )),
        ],
      ]),
    );
  }

  Widget _buildAuditCard() {
    final entries = (_audit?['entries'] as List?) ??
        (_audit?['audit_entries'] as List?) ??
        [];
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: RoyalTheme.gradientCard(),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Row(children: [
          const Icon(Icons.history, color: RoyalTheme.silver, size: 18),
          const SizedBox(width: 8),
          Text('Recent Audit', style: Theme.of(context).textTheme.titleMedium),
        ]),
        const SizedBox(height: 12),
        if (entries.isEmpty)
          Text('No audit entries yet',
              style: Theme.of(context).textTheme.bodyMedium)
        else
          ...(entries.take(5).map((e) => _auditRow(e as Map<String, dynamic>))),
      ]),
    );
  }

  Widget _auditRow(Map<String, dynamic> e) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 4),
      child: Row(children: [
        Container(
            width: 6,
            height: 6,
            decoration: const BoxDecoration(
                shape: BoxShape.circle, color: RoyalTheme.gold)),
        const SizedBox(width: 8),
        Text('${e['action'] ?? e['event'] ?? '—'}',
            style: const TextStyle(fontSize: 14, color: Colors.white)),
        const Spacer(),
        Text(
            '${e['timestamp'] ?? e['time'] ?? ''}'.length > 10
                ? '${e['timestamp'] ?? e['time'] ?? ''}'.substring(0, 10)
                : '${e['timestamp'] ?? e['time'] ?? ''}',
            style: Theme.of(context).textTheme.bodyMedium),
      ]),
    );
  }

  Widget _kvRow(String k, String v) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 3),
      child: Row(children: [
        Text('$k: ',
            style: const TextStyle(color: Color(0xFF8B949E), fontSize: 14)),
        Text(v,
            style: const TextStyle(
                color: Colors.white,
                fontSize: 14,
                fontWeight: FontWeight.w500)),
      ]),
    );
  }
}
