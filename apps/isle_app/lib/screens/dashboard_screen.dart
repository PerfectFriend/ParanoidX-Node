import 'package:flutter/material.dart';
import 'package:api_client/api_client.dart' show SimplexApiClient;
import 'package:models/models.dart' show Identity, TreasuryState;

/// Citizen Dashboard — показывает публичные данные экономики Острова
class DashboardScreen extends StatefulWidget {
  final SimplexApiClient client;
  final Identity identity;

  const DashboardScreen({super.key, required this.client, required this.identity});

  @override
  State<DashboardScreen> createState() => _DashboardScreenState();
}

class _DashboardScreenState extends State<DashboardScreen> {
  TreasuryState? _treasury;
  String? _error;
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() => _loading = true);
    try {
      final treasury = await widget.client.treasury.state();
      if (mounted) {
        setState(() {
          _treasury = treasury;
          _error = null;
          _loading = false;
        });
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _error = e.toString();
          _loading = false;
        });
      }
    }
  }

  String _fmt(dynamic value, {int decimals = 2}) {
    if (value == null) return '0';
    final num v = (value is int ? value : int.tryParse(value.toString()) ?? 0);
    return (v / 1000000000).toStringAsFixed(decimals);
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Scaffold(
      appBar: AppBar(
        title: const Text('The Isle — Dashboard'),
        actions: [
          IconButton(
            icon: const Icon(Icons.refresh),
            onPressed: _load,
            tooltip: 'Refresh',
          ),
        ],
      ),
      body: _loading
          ? const Center(child: CircularProgressIndicator())
          : _error != null
              ? Center(
                  child: Padding(
                    padding: const EdgeInsets.all(24),
                    child: Column(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        Icon(Icons.cloud_off, size: 64, color: theme.colorScheme.error),
                        const SizedBox(height: 16),
                        Text('Could not connect to node',
                            style: theme.textTheme.titleMedium),
                        const SizedBox(height: 8),
                        Text(_error!, style: theme.textTheme.bodySmall,
                            textAlign: TextAlign.center),
                        const SizedBox(height: 16),
                        FilledButton.tonalIcon(
                          onPressed: _load,
                          icon: const Icon(Icons.refresh),
                          label: const Text('Retry'),
                        ),
                      ],
                    ),
                  ),
                )
              : RefreshIndicator(
                  onRefresh: _load,
                  child: ListView(
                    padding: const EdgeInsets.all(16),
                    children: [
                      // ── Economy Overview ──
                      _SectionHeader(title: 'Economy'),
                      _InfoCard(
                        items: [
                          _InfoItem('Reserve', '${_fmt(_treasury?.reserveNg)} TLR'),
                          _InfoItem('Supply', '${_fmt(_treasury?.supplyNg)} TLR'),
                          _InfoItem('Backing Ratio',
                              '${(_treasury?.backingRatio ?? 0 * 100).toStringAsFixed(1)}%'),
                          _InfoItem('Dividend Pool',
                              '${_fmt(_treasury?.dividendPoolNg)} TLR'),
                        ],
                      ),
                      const SizedBox(height: 16),

                      // ── Treasury Info ──
                      _SectionHeader(title: 'Treasury'),
                      _InfoCard(
                        items: [
                          _InfoItem('Silver Spot',
                              '\$${_treasury?.silverSpotUsd?.toStringAsFixed(2) ?? '?'}/oz'),
                          if (_treasury?.silverReserveOz != null)
                            _InfoItem('Silver Reserve',
                                '${_treasury!.silverReserveOz!.toStringAsFixed(2)} oz'),
                          _InfoItem('Banknotes', '${_treasury?.totalBanknotes ?? '?'}'),
                          _InfoItem('Accounts', '${_treasury?.accounts ?? '?'}'),
                        ],
                      ),
                      const SizedBox(height: 16),

                      // ── Status ──
                      _SectionHeader(title: 'Status'),
                      _InfoCard(
                        items: [
                          _InfoItem('Tier', _treasury?.tier ?? 'standard'),
                          _InfoItem('Royal Status',
                              _treasury?.isRoyal == true ? '👑 Active' : 'Citizen'),
                        ],
                      ),
                      const SizedBox(height: 16),

                      // ── Actions ──
                      _SectionHeader(title: 'Quick Actions'),
                      Wrap(
                        spacing: 12,
                        runSpacing: 12,
                        children: [
                          ActionChip(
                            avatar: const Icon(Icons.wallet, size: 18),
                            label: const Text('Wallet'),
                            onPressed: () {},
                          ),
                          ActionChip(
                            avatar: const Icon(Icons.chat, size: 18),
                            label: const Text('Chat'),
                            onPressed: () {},
                          ),
                          ActionChip(
                            avatar: const Icon(Icons.radio, size: 18),
                            label: const Text('Radio'),
                            onPressed: () {},
                          ),
                          ActionChip(
                            avatar: const Icon(Icons.store, size: 18),
                            label: const Text('Market'),
                            onPressed: () {},
                          ),
                        ],
                      ),
                    ],
                  ),
                ),
    );
  }
}

// ── Reusable Widgets ──

class _SectionHeader extends StatelessWidget {
  final String title;
  const _SectionHeader({required this.title});

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 8),
      child: Text(title,
          style: Theme.of(context).textTheme.titleSmall?.copyWith(
                fontWeight: FontWeight.w600,
                color: Theme.of(context).colorScheme.primary,
              )),
    );
  }
}

class _InfoCard extends StatelessWidget {
  final List<_InfoItem> items;
  const _InfoCard({required this.items});

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          children: items.map((item) => Padding(
            padding: const EdgeInsets.symmetric(vertical: 6),
            child: Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                Text(item.label, style: theme.textTheme.bodyMedium),
                Text(item.value,
                    style: theme.textTheme.bodyMedium?.copyWith(
                      fontWeight: FontWeight.w600,
                      color: theme.colorScheme.primary,
                    )),
              ],
            ),
          )).toList(),
        ),
      ),
    );
  }
}

class _InfoItem {
  final String label;
  final String value;
  const _InfoItem(this.label, this.value);
}
