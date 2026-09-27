import 'package:flutter/material.dart';
import 'package:api_client/api_client.dart' show SimplexApiClient;
import 'package:models/models.dart' show Identity, WalletBalance, WalletTransaction, Banknote;
import 'package:widgets/widgets.dart' show NgDisplay;

/// WalletScreen manages the user's wallet - balance, transactions, tokens, and coins.
class WalletScreen extends StatefulWidget {
  final SimplexApiClient client;
  final Identity identity;

  const WalletScreen({super.key, required this.client, required this.identity});

  @override
  State<WalletScreen> createState() => _WalletScreenState();
}

class _WalletScreenState extends State<WalletScreen> with SingleTickerProviderStateMixin {
  late TabController _tabs;
  WalletBalance? _balance;
  List<WalletTransaction> _transactions = [];
  List<Banknote> _coins = [];
  bool _loading = true;
  bool _loadingTx = false;
  bool _loadingCoins = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    _tabs = TabController(length: 4, vsync: this);
    _load();
    _loadTransactions();
    _loadCoins();
  }

  @override
  void dispose() {
    _tabs.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    setState(() => _loading = true);
    try {
      final balance = await widget.client.wallet.getBalance(pubkey: widget.identity.ed25519PubKey);
      if (mounted) {
        setState(() {
          _balance = balance;
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

  Future<void> _loadTransactions() async {
    setState(() => _loadingTx = true);
    try {
      final txs = await widget.client.wallet.history(pubkey: widget.identity.ed25519PubKey, limit: 50);
      if (mounted) {
        setState(() {
          _transactions = txs;
          _loadingTx = false;
        });
      }
    } catch (e) {
      if (mounted) setState(() => _loadingTx = false);
    }
  }

  Future<void> _loadCoins() async {
    setState(() => _loadingCoins = true);
    try {
      // Use wallet history to find coins
      final history = await widget.client.wallet.history(pubkey: widget.identity.ed25519PubKey, limit: 100);
      final coins = history
          .where((tx) => tx.type == 'coin_purchase' || tx.type == 'coin_redeem' || tx.type == 'banknote_created')
          .map((tx) => Banknote(
                serial: tx.id,
                denominationNg: tx.amountNg,
                rarity: 'common',
                holder: widget.identity.ed25519PubKey,
                status: 'owned',
              ))
          .toList();

      if (mounted) {
        setState(() {
          _coins = coins;
          _loadingCoins = false;
        });
      }
    } catch (e) {
      if (mounted) setState(() => _loadingCoins = false);
    }
  }

  Future<void> _sendNt() async {
    final toCtrl = TextEditingController();
    final amountCtrl = TextEditingController();
    final memoCtrl = TextEditingController();

    final result = await showDialog<Map<String, dynamic>>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Send NT'),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            TextField(
              controller: toCtrl,
              decoration: const InputDecoration(labelText: 'Recipient Public Key (Ed25519)'),
            ),
            TextField(
              controller: amountCtrl,
              decoration: const InputDecoration(labelText: 'Amount (ng)'),
              keyboardType: TextInputType.number,
            ),
            TextField(
              controller: memoCtrl,
              decoration: const InputDecoration(labelText: 'Memo (optional)'),
            ),
          ],
        ),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx), child: const Text('Cancel')),
          FilledButton(onPressed: () => Navigator.pop(ctx, {'to': toCtrl.text, 'amount': amountCtrl.text, 'memo': memoCtrl.text}), child: const Text('Send')),
        ],
      ),
    );

    if (result == null) return;

    try {
      final amount = int.tryParse(result['amount']) ?? 0;
      final res = await widget.client.wallet.transfer(from: widget.identity.ed25519PubKey, to: result['to'], amountNg: amount, memo: result['memo'].isNotEmpty ? result['memo'] : null);
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Sent: ${res.status}')));
        _load();
        _loadTransactions();
      }
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Send failed: $e')));
    }
  }

  Future<void> _receive() async {
    try {
      final info = await widget.client.wallet.receive(widget.identity.ed25519PubKey);
      if (mounted) {
        showDialog(
          context: context,
          builder: (ctx) => AlertDialog(
            title: const Text('Receive NT'),
            content: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text('Address: ${info.address}', style: const TextStyle(fontFamily: 'monospace', fontSize: 12)),
                const SizedBox(height: 8),
                Text('QR Code: ${info.qrCode}'),
                const SizedBox(height: 8),
                FilledButton(onPressed: () => Navigator.pop(ctx), child: const Text('Close')),
              ],
            ),
          ),
        );
      }
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Receive failed: $e')));
    }
  }

  Future<void> _mintCoin() async {
    final amountCtrl = TextEditingController(text: '1000000000'); // 1 NT

    final amount = await showDialog<int>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Mint Coin'),
        content: TextField(
          controller: amountCtrl,
          decoration: const InputDecoration(labelText: 'Amount (ng)'),
          keyboardType: TextInputType.number,
          autofocus: true,
        ),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx), child: const Text('Cancel')),
          FilledButton(onPressed: () => Navigator.pop(ctx, int.tryParse(amountCtrl.text)), child: const Text('Mint')),
        ],
      ),
    );

    if (amount == null) return;

    try {
      final res = await widget.client.wallet.mint(holder: widget.identity.ed25519PubKey, amountNg: amount);
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Minted: ${res.status}')));
        _load();
        _loadTransactions();
        _loadCoins();
      }
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Mint failed: $e')));
    }
  }

  Future<void> _redeemCoin(Banknote coin) async {
    final confirm = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Redeem Coin'),
        content: Text('Redeem "${coin.serial}" for ${_formatNt(coin.denominationNg)} NT?'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('Cancel')),
          FilledButton(onPressed: () => Navigator.pop(ctx, true), child: const Text('Redeem')),
        ],
      ),
    );
    if (confirm != true) return;

    try {
      await widget.client.wallet.redeem(assetId: coin.serial, holder: widget.identity.ed25519PubKey);
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Coin redeemed!')));
        _load();
        _loadTransactions();
        _loadCoins();
      }
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Redeem failed: $e')));
    }
  }

  Future<void> _transferCoin(Banknote coin) async {
    final toCtrl = TextEditingController();

    final to = await showDialog<String>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Transfer Coin'),
        content: TextField(
          controller: toCtrl,
          decoration: const InputDecoration(labelText: 'Recipient Public Key (Ed25519)'),
          autofocus: true,
        ),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx), child: const Text('Cancel')),
          FilledButton(onPressed: () => Navigator.pop(ctx, toCtrl.text.trim()), child: const Text('Transfer')),
        ],
      ),
    );

    if (to == null || to.isEmpty) return;

    try {
      await widget.client.wallet.transfer(
        from: widget.identity.ed25519PubKey,
        to: to,
        amountNg: 0,
        memo: 'Transfer coin ${coin.serial}',
      );
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Transfer initiated')));
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Transfer failed: $e')));
    }
  }

  String _formatNt(int ng) => (ng / 1000000000).toStringAsFixed(9);

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final ntColor = theme.colorScheme.primary;

    if (_loading) {
      return const Scaffold(body: Center(child: CircularProgressIndicator()));
    }

    if (_error != null) {
      return Scaffold(
        appBar: AppBar(title: const Text('Wallet'), actions: [IconButton(icon: const Icon(Icons.refresh), onPressed: _load)]),
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

    final balance = _balance;
    final availableNg = balance?.availableNg ?? 0;
    final stakedNg = balance?.stakedNg ?? 0;
    final silverNg = balance?.silverNg ?? 0;
    final totalValueUsd = balance?.totalValueUsd ?? 0.0;

    return Scaffold(
      appBar: AppBar(
        title: const Text('Wallet'),
        actions: [
          if (_loading)
            const Padding(
              padding: EdgeInsets.all(16),
              child: SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2)),
            )
          else
            IconButton(icon: const Icon(Icons.refresh), onPressed: _load, tooltip: 'Refresh'),
        ],
        bottom: TabBar(
          controller: _tabs,
          isScrollable: true,
          tabs: const [
            Tab(icon: Icon(Icons.account_balance_wallet), text: 'Balance'),
            Tab(icon: Icon(Icons.history), text: 'Transactions'),
            Tab(icon: Icon(Icons.monetization_on), text: 'Coins'),
            Tab(icon: Icon(Icons.swap_horiz), text: 'Actions'),
          ],
        ),
      ),
      body: RefreshIndicator(
        onRefresh: _load,
        child: TabBarView(
          controller: _tabs,
          children: [
            // Balance Tab
            SingleChildScrollView(
              padding: const EdgeInsets.all(16),
              physics: const AlwaysScrollableScrollPhysics(),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  // Main Balance Card
                  Card(
                    elevation: 2,
                    child: Padding(
                      padding: const EdgeInsets.all(20),
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text('Available Balance', style: theme.textTheme.labelLarge?.copyWith(color: theme.colorScheme.onSurfaceVariant)),
                          const SizedBox(height: 8),
                          NgDisplay(ngAmount: availableNg, style: theme.textTheme.displayMedium?.copyWith(color: ntColor, fontWeight: FontWeight.bold)),
                          const SizedBox(height: 8),
                          Text('${_formatNt(availableNg)} NT', style: theme.textTheme.bodyLarge?.copyWith(color: theme.colorScheme.onSurfaceVariant)),
                        ],
                      ),
                    ),
                  ),
                  const SizedBox(height: 16),
                  // Breakdown
                  Row(
                    children: [
                      Expanded(
                        child: Card(
                          child: Padding(
                            padding: const EdgeInsets.all(16),
                            child: Column(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: [
                                Text('Staked', style: theme.textTheme.labelLarge?.copyWith(color: theme.colorScheme.onSurfaceVariant)),
                                const SizedBox(height: 4),
                                NgDisplay(ngAmount: stakedNg, style: theme.textTheme.titleLarge?.copyWith(color: theme.colorScheme.primary)),
                              ],
                            ),
                          ),
                        ),
                      ),
                      const SizedBox(width: 12),
                      Expanded(
                        child: Card(
                          child: Padding(
                            padding: const EdgeInsets.all(16),
                            child: Column(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: [
                                Text('Silver Backed', style: theme.textTheme.labelLarge?.copyWith(color: theme.colorScheme.onSurfaceVariant)),
                                const SizedBox(height: 4),
                                NgDisplay(ngAmount: silverNg, style: theme.textTheme.titleLarge?.copyWith(color: Colors.amber)),
                              ],
                            ),
                          ),
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 16),
                  // USD Value
                  Card(
                    child: Padding(
                      padding: const EdgeInsets.all(16),
                      child: Row(
                        mainAxisAlignment: MainAxisAlignment.spaceBetween,
                        children: [
                          Text('Estimated USD Value', style: theme.textTheme.labelLarge?.copyWith(color: theme.colorScheme.onSurfaceVariant)),
                          Text('\$${totalValueUsd.toStringAsFixed(2)}', style: theme.textTheme.titleLarge?.copyWith(color: Colors.green)),
                        ],
                      ),
                    ),
                  ),
                  const SizedBox(height: 24),
                  // Action Buttons
                  Row(
                    children: [
                      Expanded(
                        child: FilledButton.icon(
                          icon: const Icon(Icons.send),
                          label: const Text('Send NT'),
                          onPressed: _sendNt,
                          style: FilledButton.styleFrom(padding: const EdgeInsets.symmetric(vertical: 14)),
                        ),
                      ),
                      const SizedBox(width: 12),
                      Expanded(
                        child: OutlinedButton.icon(
                          icon: const Icon(Icons.qr_code),
                          label: const Text('Receive'),
                          onPressed: _receive,
                          style: OutlinedButton.styleFrom(padding: const EdgeInsets.symmetric(vertical: 14)),
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 12),
                  Row(
                    children: [
                      Expanded(
                        child: FilledButton.icon(
                          icon: const Icon(Icons.monetization_on),
                          label: const Text('Mint Coin (1 NT)'),
                          onPressed: _mintCoin,
                          style: FilledButton.styleFrom(
                            padding: const EdgeInsets.symmetric(vertical: 14),
                            backgroundColor: Colors.amber.shade700,
                          ),
                        ),
                      ),
                    ],
                  ),
                ],
              ),
            ),
            // Transactions Tab
            _loadingTx
                ? const Center(child: CircularProgressIndicator())
                : _transactions.isEmpty
                    ? const Center(child: Text('No transactions yet'))
                    : RefreshIndicator(
                        onRefresh: _loadTransactions,
                        child: ListView.builder(
                          padding: const EdgeInsets.all(8),
                          itemCount: _transactions.length,
                          itemBuilder: (context, index) {
                            final tx = _transactions[index];
                            final isSent = tx.fromPubkey == widget.identity.ed25519PubKey;
                            final isReceived = tx.toPubkey == widget.identity.ed25519PubKey;
                            final color = isSent ? Colors.red : (isReceived ? Colors.green : theme.colorScheme.primary);

                            return Card(
                              margin: const EdgeInsets.symmetric(vertical: 4, horizontal: 8),
                              child: ListTile(
                                leading: CircleAvatar(
                                  backgroundColor: color.withValues(alpha: 0.15),
                                  child: Icon(isSent ? Icons.arrow_upward : Icons.arrow_downward, color: color),
                                ),
                                title: Text('${isSent ? 'Sent' : 'Received'} ${_formatNt(tx.amountNg)} NT'),
                                subtitle: Column(
                                  crossAxisAlignment: CrossAxisAlignment.start,
                                  children: [
                                    Text('${tx.type} • ${tx.status}', style: theme.textTheme.bodySmall),
                                    Text('${tx.fromPubkey?.substring(0, 12) ?? '...'} → ${tx.toPubkey?.substring(0, 12) ?? '...'}', style: theme.textTheme.bodySmall),
                                  ],
                                ),
                                trailing: Text(tx.createdAt, style: theme.textTheme.bodySmall),
                              ),
                            );
                          },
                        ),
                      ),
            // Coins Tab
            _loadingCoins
                ? const Center(child: CircularProgressIndicator())
                : _coins.isEmpty
                    ? const Center(child: Text('No coins in vault. Mint or buy coins to get started.'))
                    : RefreshIndicator(
                        onRefresh: _loadCoins,
                        child: ListView.builder(
                          padding: const EdgeInsets.all(16),
                          itemCount: _coins.length,
                          itemBuilder: (context, index) {
                            final coin = _coins[index];
                            return Card(
                              margin: const EdgeInsets.symmetric(vertical: 6),
                              child: ListTile(
                                leading: Icon(Icons.monetization_on, color: ntColor, size: 32),
                                title: Text('Serial: ${coin.serial}'),
                                subtitle: Column(
                                  crossAxisAlignment: CrossAxisAlignment.start,
                                  children: [
                                    NgDisplay(ngAmount: coin.denominationNg),
                                    Text('Rarity: ${coin.rarityLabel}', style: theme.textTheme.bodySmall),
                                  ],
                                ),
                                trailing: PopupMenuButton<String>(
                                  onSelected: (value) {
                                    if (value == 'redeem') {
                                      _redeemCoin(coin);
                                    } else if (value == 'transfer') {
                                      _transferCoin(coin);
                                    }
                                  },
                                  itemBuilder: (context) => [
                                    const PopupMenuItem(value: 'redeem', child: Text('Redeem for NT')),
                                    const PopupMenuItem(value: 'transfer', child: Text('Transfer')),
                                  ],
                                ),
                              ),
                            );
                          },
                        ),
                      ),
            // Actions Tab
            SingleChildScrollView(
              padding: const EdgeInsets.all(16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text('Quick Actions', style: theme.textTheme.headlineSmall),
                  const SizedBox(height: 16),
                  _ActionCard(
                    icon: Icons.send,
                    title: 'Send NT',
                    subtitle: 'Transfer NT to another user',
                    color: theme.colorScheme.primary,
                    onTap: _sendNt,
                  ),
                  _ActionCard(
                    icon: Icons.qr_code,
                    title: 'Receive NT',
                    subtitle: 'Get your address and QR code',
                    color: theme.colorScheme.secondary,
                    onTap: _receive,
                  ),
                  _ActionCard(
                    icon: Icons.monetization_on,
                    title: 'Mint Coin',
                    subtitle: 'Create a silver-backed coin (1 NT = 1 coin)',
                    color: Colors.amber.shade700,
                    onTap: _mintCoin,
                  ),
                  _ActionCard(
                    icon: Icons.swap_horiz,
                    title: 'Swap Tokens',
                    subtitle: 'Exchange NT for other tokens',
                    color: theme.colorScheme.tertiary,
                    onTap: () => ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Swap coming soon'))),
                  ),
                  _ActionCard(
                    icon: Icons.account_balance,
                    title: 'Stake NT',
                    subtitle: 'Earn rewards by staking',
                    color: Colors.blue,
                    onTap: () => ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Staking coming soon'))),
                  ),
                  const SizedBox(height: 24),
                  Text('Coin Management', style: theme.textTheme.headlineSmall),
                  const SizedBox(height: 16),
                  _ActionCard(
                    icon: Icons.add_card,
                    title: 'List Coin for Sale',
                    subtitle: 'Sell your coins on the marketplace',
                    color: Colors.green,
                    onTap: () => _tabs.animateTo(2), // Switch to Coins tab
                  ),
                  _ActionCard(
                    icon: Icons.swap_horiz,
                    title: 'Transfer Coin',
                    subtitle: 'Send a coin to another user',
                    color: Colors.orange,
                    onTap: () => _tabs.animateTo(2), // Switch to Coins tab
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _ActionCard extends StatelessWidget {
  final IconData icon;
  final String title;
  final String subtitle;
  final Color color;
  final VoidCallback onTap;

  const _ActionCard({
    required this.icon,
    required this.title,
    required this.subtitle,
    required this.color,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return Card(
      margin: const EdgeInsets.symmetric(vertical: 6),
      child: ListTile(
        leading: CircleAvatar(backgroundColor: color.withValues(alpha: 0.15), child: Icon(icon, color: color)),
        title: Text(title),
        subtitle: Text(subtitle),
        trailing: const Icon(Icons.chevron_right),
        onTap: onTap,
      ),
    );
  }
}