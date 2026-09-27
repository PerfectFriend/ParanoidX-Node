import 'dart:async';
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../theme.dart';
import '../main.dart';

/// CommunicationsScreen manages SimpleX chat bridge communications.
class CommunicationsScreen extends StatefulWidget {
  const CommunicationsScreen({super.key});
  @override
  State<CommunicationsScreen> createState() => _CommunicationsScreenState();
}

class _CommunicationsScreenState extends State<CommunicationsScreen> {
  final _msgCtrl = TextEditingController();
  Map<String, dynamic>? _status;
  Timer? _timer;
  bool _isLoading = true;
  String? _error;

  @override
  void initState() {
    super.initState();
    _refresh();
    _timer = Timer.periodic(const Duration(seconds: 30), (_) => _refresh());
  }

  @override
  void dispose() {
    _timer?.cancel();
    _msgCtrl.dispose();
    super.dispose();
  }

  Future<void> _refresh() async {
    try {
      final s = await context.read<AppState>().api.chatStatus();
      if (mounted) {
        setState(() {
          _status = s;
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

  Future<void> _broadcast() async {
    if (_msgCtrl.text.trim().isEmpty) return;
    try {
      final res =
          await context.read<AppState>().api.chatBroadcast(_msgCtrl.text);
      if (mounted) {
        _msgCtrl.clear();
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(
          content:
              Text(res['ok'] == true ? 'Broadcast sent' : 'Failed'),
          backgroundColor: res['ok'] == true
              ? RoyalTheme.green.withAlpha(40)
              : RoyalTheme.red.withAlpha(40),
        ));
      }
    } catch (e) {
      if (mounted)
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(
            content: Text('Error: $e'),
            backgroundColor: RoyalTheme.red.withAlpha(40)));
    }
  }

  Future<void> _sendTreasuryAlert() async {
    try {
      final res = await context.read<AppState>().api.chatTreasuryAlert();
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(
          content: Text(res['ok'] == true ? 'Treasury alert sent' : 'Failed'),
          backgroundColor: res['ok'] == true
              ? RoyalTheme.green.withAlpha(40)
              : RoyalTheme.red.withAlpha(40),
        ));
      }
    } catch (e) {
      if (mounted)
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(
            content: Text('Error: $e'),
            backgroundColor: RoyalTheme.red.withAlpha(40)));
    }
  }

  @override
  Widget build(BuildContext context) {
    final connected =
        _status?['bridge'] == true || _status?['bridge_connected'] == true || _status?['connected'] == true;
    final msgCount = _status?['messages'] ?? _status?['message_count'] ?? _status?['total_messages'] ?? 0;
    final contacts = _status?['contacts'] ?? 0;
    final simplexCmd = _status?['simplex_cmd'] == true;
    final reconnectCount = _status?['reconnect_count'] ?? 0;
    return Scaffold(
      appBar: AppBar(
        title: Row(children: [
          Container(
            padding: const EdgeInsets.all(6),
            decoration: BoxDecoration(
                color: RoyalTheme.teal.withAlpha(30),
                borderRadius: BorderRadius.circular(8)),
            child: const Icon(Icons.chat, color: RoyalTheme.teal, size: 20),
          ),
          const SizedBox(width: 12),
          const Text('Communications'),
        ]),
        actions: [
          IconButton(icon: const Icon(Icons.refresh), onPressed: _refresh),
          Container(
            margin: const EdgeInsets.only(right: 12),
            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
            decoration: BoxDecoration(
              color: (connected ? RoyalTheme.green : RoyalTheme.red)
                  .withAlpha(30),
              borderRadius: BorderRadius.circular(20),
            ),
            child: Text(connected ? 'Bridge Online' : 'Bridge Offline',
                style: TextStyle(
                    fontSize: 15,
                    color: connected ? RoyalTheme.green : RoyalTheme.red,
                    fontWeight: FontWeight.w600)),
          ),
        ],
      ),
      body: _isLoading
          ? const Center(
              child: CircularProgressIndicator(color: RoyalTheme.gold))
          : _error != null && _status == null
              ? _buildErrorView()
              : RefreshIndicator(
                  onRefresh: _refresh,
                  child: ListView(
                    padding: const EdgeInsets.all(16),
                    children: [
                      _buildBridgeCard(connected, msgCount, contacts,
                          simplexCmd, reconnectCount),
                      const SizedBox(height: 16),
                      _buildBroadcastCard(),
                      const SizedBox(height: 16),
                      _buildTreasuryAlertCard(),
                      if (_error != null) ...[
                        const SizedBox(height: 12),
                        _buildErrorBanner(),
                      ],
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
            Text('Cannot connect to bridge',
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

  Widget _buildErrorBanner() {
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: RoyalTheme.red.withAlpha(20),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: RoyalTheme.red.withAlpha(60)),
      ),
      child: Row(children: [
        const Icon(Icons.warning_amber, color: RoyalTheme.red, size: 18),
        const SizedBox(width: 8),
        Expanded(
          child: Text('Background refresh failed: $_error',
              style: const TextStyle(color: RoyalTheme.red, fontSize: 13)),
        ),
        GestureDetector(
          onTap: () {
            setState(() => _isLoading = true);
            _refresh();
          },
          child: const Icon(Icons.refresh, color: RoyalTheme.gold, size: 20),
        ),
      ]),
    );
  }

  Widget _buildBridgeCard(bool connected, dynamic msgCount, dynamic contacts,
      bool simplexCmd, dynamic reconnectCount) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: RoyalTheme.gradientCard(),
      child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
        Row(children: [
          const Icon(Icons.wifi, size: 18,
              color: RoyalTheme.teal),
          const SizedBox(width: 8),
          Text('SimpleX Bridge',
              style: Theme.of(context).textTheme.titleMedium),
          const Spacer(),
          Container(
            padding:
                const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
            decoration: BoxDecoration(
              color: (connected ? RoyalTheme.green : RoyalTheme.red)
                  .withAlpha(25),
              borderRadius: BorderRadius.circular(10),
              border: Border.all(
                  color: (connected ? RoyalTheme.green : RoyalTheme.red)
                      .withAlpha(60)),
            ),
            child: Text(connected ? 'Live' : 'Offline',
                style: TextStyle(
                    fontSize: 14,
                    color:
                        connected ? RoyalTheme.green : RoyalTheme.red)),
          ),
        ]),
        const SizedBox(height: 12),
        Row(children: [
          _miniStat('Messages', '$msgCount', RoyalTheme.accent),
          const SizedBox(width: 24),
          _miniStat('Contacts', '$contacts', RoyalTheme.gold),
          const SizedBox(width: 24),
          _miniStat('Reconnects', '$reconnectCount',
              reconnectCount > 0 ? RoyalTheme.orange : RoyalTheme.green),
        ]),
        const SizedBox(height: 8),
        Row(children: [
          Icon(Icons.check_circle,
              size: 14,
              color: simplexCmd
                  ? RoyalTheme.green
                  : RoyalTheme.red.withAlpha(100)),
          const SizedBox(width: 6),
          Text(simplexCmd ? 'SimpleX CMD active' : 'SimpleX CMD inactive',
              style: TextStyle(
                  fontSize: 13,
                  color: simplexCmd
                      ? RoyalTheme.green
                      : RoyalTheme.red.withAlpha(100))),
        ]),
      ]),
    );
  }

  Widget _buildBroadcastCard() {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: RoyalTheme.glowBorder(color: RoyalTheme.teal),
      child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
        Row(children: [
          const Icon(Icons.campaign, color: RoyalTheme.orange, size: 20),
          const SizedBox(width: 8),
          Text('Royal Broadcast',
              style: Theme.of(context).textTheme.titleMedium),
        ]),
        const SizedBox(height: 12),
        TextField(
          controller: _msgCtrl,
          maxLines: 4,
          decoration: const InputDecoration(
            hintText: 'Type your broadcast message to all contacts...',
          ),
        ),
        const SizedBox(height: 12),
        Align(
          alignment: Alignment.centerRight,
          child: ElevatedButton.icon(
            icon: const Icon(Icons.send, size: 18),
            label: const Text('Broadcast'),
            onPressed: _broadcast,
          ),
        ),
      ]),
    );
  }

  Widget _buildTreasuryAlertCard() {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: RoyalTheme.gradientCard(),
      child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
        Row(children: [
          const Icon(Icons.notifications, color: RoyalTheme.gold, size: 18),
          const SizedBox(width: 8),
          Text('Treasury Alert',
              style: Theme.of(context).textTheme.titleMedium),
        ]),
        const SizedBox(height: 8),
        Text(
            'Send an urgent treasury notification to all SimpleX contacts.',
            style: Theme.of(context).textTheme.bodyMedium),
        const SizedBox(height: 12),
        ElevatedButton.icon(
          icon: const Icon(Icons.warning, size: 18),
          label: const Text('Send Treasury Alert'),
          style: ElevatedButton.styleFrom(
              backgroundColor: RoyalTheme.gold,
              foregroundColor: Colors.black),
          onPressed: _sendTreasuryAlert,
        ),
      ]),
    );
  }

  Widget _miniStat(String label, String value, Color color) {
    return Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
      Text(value,
          style: TextStyle(
              fontSize: 16,
              fontWeight: FontWeight.w700,
              color: color)),
      Text(label, style: Theme.of(context).textTheme.bodyMedium),
    ]);
  }
}
