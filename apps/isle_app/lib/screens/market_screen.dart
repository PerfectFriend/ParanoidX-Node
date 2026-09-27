import 'package:flutter/material.dart';
import 'package:api_client/api_client.dart' show SimplexApiClient;
import 'package:models/models.dart' show Identity, MarketListing, MarketOrder, Banknote;
import 'package:widgets/widgets.dart' show NgDisplay;

/// MarketScreen - marketplace for buying/selling banknotes and coins
class MarketScreen extends StatefulWidget {
  final SimplexApiClient client;
  final Identity identity;

  const MarketScreen({super.key, required this.client, required this.identity});

  @override
  State<MarketScreen> createState() => _MarketScreenState();
}

class _MarketScreenState extends State<MarketScreen> with SingleTickerProviderStateMixin {
  late TabController _tabs;
  List<MarketListing> _listings = [];
  List<MarketOrder> _myOrders = [];
  List<Banknote> _myCoins = [];
  bool _loading = true;
  bool _loadingMy = false;
  bool _loadingCoins = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    _tabs = TabController(length: 3, vsync: this);
    _load();
    _loadMyOrders();
    _loadMyCoins();
  }

  @override
  void dispose() {
    _tabs.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    setState(() => _loading = true);
    try {
      final listings = await widget.client.market.list(limit: 50);
      if (mounted) {
        setState(() {
          _listings = listings;
          _loading = false;
          _error = null;
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

  Future<void> _loadMyOrders() async {
    setState(() => _loadingMy = true);
    try {
      final orders = await widget.client.market.orders(pubkey: widget.identity.ed25519PubKey, limit: 50);
      if (mounted) {
        setState(() {
          _myOrders = orders;
          _loadingMy = false;
        });
      }
    } catch (e) {
      if (mounted) setState(() => _loadingMy = false);
    }
  }

  Future<void> _loadMyCoins() async {
    setState(() => _loadingCoins = true);
    try {
      // Use wallet history to find coins owned by this identity
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
          _myCoins = coins;
          _loadingCoins = false;
        });
      }
    } catch (e) {
      if (mounted) setState(() => _loadingCoins = false);
    }
  }

  Future<void> _buyListing(MarketListing listing) async {
    final confirm = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Confirm Purchase'),
        content: Text('Buy "${listing.serial}" for ${_formatNt(listing.priceNg)} NT?'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('Cancel')),
          FilledButton(onPressed: () => Navigator.pop(ctx, true), child: const Text('Buy')),
        ],
      ),
    );
    if (confirm != true) return;

    try {
      await widget.client.market.buy(listingId: listing.id, buyerPubkey: widget.identity.ed25519PubKey);
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Purchase successful!')));
        _load();
        _loadMyOrders();
        _loadMyCoins();
      }
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Failed: $e')));
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
        appBar: AppBar(title: const Text('Market'), actions: [IconButton(icon: const Icon(Icons.refresh), onPressed: _load)]),
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

    return Scaffold(
      appBar: AppBar(
        title: const Text('Market'),
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
            Tab(icon: Icon(Icons.store), text: 'Marketplace'),
            Tab(icon: Icon(Icons.receipt_long), text: 'My Orders'),
            Tab(icon: Icon(Icons.monetization_on), text: 'My Coins'),
          ],
        ),
      ),
      body: TabBarView(
        controller: _tabs,
        children: [
          // Marketplace Tab
          RefreshIndicator(
            onRefresh: _load,
            child: _listings.isEmpty
                ? const Center(child: Text('No listings available. Pull to refresh.'))
                : ListView.builder(
                    padding: const EdgeInsets.all(12),
                    itemCount: _listings.length,
                    itemBuilder: (context, index) {
                      final listing = _listings[index];
                      return Card(
                        margin: const EdgeInsets.symmetric(vertical: 6),
                        child: ListTile(
                          leading: Icon(Icons.monetization_on, color: ntColor, size: 32),
                          title: Text(listing.serial, style: const TextStyle(fontFamily: 'monospace')),
                          subtitle: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              NgDisplay(ngAmount: listing.denominationNg),
                              Text('Grade: ${listing.grade}  •  Seller: ${listing.seller.substring(0, 12)}...'),
                            ],
                          ),
                          trailing: Column(
                            mainAxisAlignment: MainAxisAlignment.center,
                            crossAxisAlignment: CrossAxisAlignment.end,
                            children: [
                              NgDisplay(ngAmount: listing.priceNg, style: theme.textTheme.titleMedium?.copyWith(color: ntColor, fontWeight: FontWeight.bold)),
                              Container(
                                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
                                decoration: BoxDecoration(
                                  color: listing.available ? Colors.green.shade100 : Colors.red.shade100,
                                  borderRadius: BorderRadius.circular(4),
                                ),
                                child: Text(
                                  listing.available ? 'AVAILABLE' : 'SOLD',
                                  style: TextStyle(
                                    color: listing.available ? Colors.green.shade700 : Colors.red.shade700,
                                    fontWeight: FontWeight.bold,
                                    fontSize: 11,
                                  ),
                                ),
                              ),
                            ],
                          ),
                          onTap: listing.available ? () => _buyListing(listing) : null,
                        ),
                      );
                    },
                  ),
          ),
          // My Orders Tab
          _loadingMy
              ? const Center(child: CircularProgressIndicator())
              : _myOrders.isEmpty
                  ? const Center(child: Text('No orders yet. Buy coins to see them here.'))
                  : RefreshIndicator(
                      onRefresh: _loadMyOrders,
                      child: ListView.builder(
                        padding: const EdgeInsets.all(12),
                        itemCount: _myOrders.length,
                        itemBuilder: (context, index) {
                          final order = _myOrders[index];
                          final isBuyer = order.buyer == widget.identity.ed25519PubKey;
                          final color = isBuyer ? Colors.green : Colors.orange;

                          return Card(
                            margin: const EdgeInsets.symmetric(vertical: 6),
                            child: ListTile(
                              leading: CircleAvatar(
                                backgroundColor: color.withValues(alpha: 0.15),
                                child: Icon(isBuyer ? Icons.shopping_cart : Icons.sell, color: color),
                              ),
                              title: Text('Listing: ${order.listingId}'),
                              subtitle: Column(
                                crossAxisAlignment: CrossAxisAlignment.start,
                                children: [
                                  Text('${isBuyer ? 'Bought' : 'Sold'} for ${_formatNt(order.amountNg)} NT', style: theme.textTheme.bodySmall),
                                  Text('Status: ${order.status}', style: theme.textTheme.bodySmall?.copyWith(color: order.status == 'completed' ? Colors.green : Colors.orange)),
                                ],
                              ),
                              trailing: Text(order.createdAt, style: theme.textTheme.bodySmall),
                            ),
                          );
                        },
                      ),
                    ),
          // My Coins Tab
          _loadingCoins
              ? const Center(child: CircularProgressIndicator())
              : _myCoins.isEmpty
                  ? const Center(child: Text('No coins in vault. Buy or mint coins to get started.'))
                  : RefreshIndicator(
                      onRefresh: _loadMyCoins,
                      child: ListView.builder(
                        padding: const EdgeInsets.all(16),
                        itemCount: _myCoins.length,
                        itemBuilder: (context, index) {
                          final coin = _myCoins[index];
                          return Card(
                            margin: const EdgeInsets.symmetric(vertical: 6),
                            child: ListTile(
                              leading: Icon(Icons.monetization_on, color: ntColor, size: 32),
                              title: Text('Serial: ${coin.serial}', style: const TextStyle(fontFamily: 'monospace')),
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
        ],
      ),
    );
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
        _loadMyCoins();
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
}