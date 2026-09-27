import 'dart:async';
import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:api_client/api_client.dart' show SimplexApiClient;

/// ParanoidXScreen manages the ParanoidX multi-layer proxy chain management interface.
class ParanoidXScreen extends StatefulWidget {
  final SimplexApiClient client;

  const ParanoidXScreen({super.key, required this.client});

  @override
  State<ParanoidXScreen> createState() => _ParanoidXScreenState();
}

class _ParanoidXScreenState extends State<ParanoidXScreen> {
  Map<String, dynamic> _status = {};
  Map<String, dynamic> _config = {};
  Map<String, dynamic> _chainState = {};
  Map<String, dynamic> _testResults = {};
  List<dynamic> _vpnProfiles = [];
  String _activeProfile = '';
  Timer? _pollTimer;
  String _actionMsg = '';

  // Layer toggle state (loaded from config)
  bool _v2rayEnabled = true;
  bool _vpnEnabled = true;
  bool _torEnabled = true;

  @override
  void initState() {
    super.initState();
    _refreshAll();
    _pollTimer = Timer.periodic(const Duration(seconds: 30), (_) => _poll());
  }

  @override
  void dispose() {
    _pollTimer?.cancel();
    super.dispose();
  }

  void _poll() {
    _loadStatus();
    _loadChainState();
  }

  Future<void> _refreshAll() async {
    await Future.wait([
      _loadStatus(),
      _loadConfig(),
      _loadChainState(),
      _loadVPNProfiles(),
    ]);
  }

  Future<void> _loadStatus() async {
    final s = await widget.client.paranoidx.getStatus();
    if (mounted) setState(() => _status = s);
  }

  Future<void> _loadConfig() async {
    final c = await widget.client.paranoidx.getConfig();
    if (mounted) setState(() {
      _config = c;
      _v2rayEnabled = c['v2ray_enabled'] as bool? ?? true;
      _vpnEnabled = c['vpn_enabled'] as bool? ?? true;
      _torEnabled = c['tor_enabled'] as bool? ?? true;
    });
  }

  Future<void> _saveConfig() async {
    _setAction('Saving settings...');
    await widget.client.paranoidx.updateConfig(v2ray: _v2rayEnabled, vpn: _vpnEnabled, tor: _torEnabled);
    await _loadConfig();
    _setAction('Settings saved');
  }

  Future<void> _loadChainState() async {
    final c = await widget.client.paranoidx.getChainState();
    if (mounted) setState(() => _chainState = c);
  }

  Future<void> _loadVPNProfiles() async {
    final p = await widget.client.paranoidx.getVPNProfiles();
    if (mounted) setState(() {
      _vpnProfiles = (p['profiles'] as List<dynamic>?) ?? [];
      _activeProfile = (p['active'] as String?) ?? '';
    });
  }

  void _setAction(String msg) {
    setState(() => _actionMsg = msg);
    Future.delayed(const Duration(seconds: 4), () {
      if (mounted) setState(() => _actionMsg = '');
    });
  }

  Future<void> _buildChain() async {
    _setAction('Building proxy chain...');
    await widget.client.paranoidx.buildChain();
    await _refreshAll();
    _setAction('Chain build complete');
  }

  Future<void> _teardownChain() async {
    _setAction('Tearing down proxy chain...');
    await widget.client.paranoidx.teardownChain();
    await _refreshAll();
    _setAction('Chain torn down');
  }

  Future<void> _runTest() async {
    _setAction('Running end-to-end chain test...');
    final t = await widget.client.paranoidx.testChain();
    if (mounted) setState(() => _testResults = t);
    _setAction('Test complete');
  }

  Future<void> _showVPNAddDialog() async {
    final nameCtrl = TextEditingController();
    final descCtrl = TextEditingController();
    final configCtrl = TextEditingController();
    await showDialog(context: context, builder: (ctx) => AlertDialog(
      backgroundColor: Colors.grey[900],
      title: const Text('Add VPN Profile', style: TextStyle(fontSize: 20)),
      content: SingleChildScrollView(child: Column(mainAxisSize: MainAxisSize.min, children: [
        TextField(controller: nameCtrl, style: const TextStyle(fontSize: 18, color: Colors.white),
          decoration: InputDecoration(labelText: 'Profile name', labelStyle: const TextStyle(fontSize: 18),
            border: OutlineInputBorder(borderRadius: BorderRadius.circular(8)), filled: true, fillColor: Colors.grey.shade800)),
        const SizedBox(height: 8),
        TextField(controller: descCtrl, style: const TextStyle(fontSize: 18, color: Colors.white),
          decoration: InputDecoration(labelText: 'Description', labelStyle: const TextStyle(fontSize: 18),
            border: OutlineInputBorder(borderRadius: BorderRadius.circular(8)), filled: true, fillColor: Colors.grey.shade800)),
        const SizedBox(height: 8),
        TextField(controller: configCtrl, maxLines: 8, style: const TextStyle(fontSize: 14, color: Colors.white, fontFamily: 'monospace'),
          decoration: InputDecoration(labelText: 'WireGuard config content', labelStyle: const TextStyle(fontSize: 18),
            border: OutlineInputBorder(borderRadius: BorderRadius.circular(8)), filled: true, fillColor: Colors.grey.shade800)),
      ])),
      actions: [
        TextButton(onPressed: () => Navigator.pop(ctx), child: const Text('Cancel', style: TextStyle(fontSize: 18))),
        ElevatedButton(onPressed: () async {
          Navigator.pop(ctx);
          await widget.client.paranoidx.addVPNProfile(nameCtrl.text.trim(), descCtrl.text.trim(), configCtrl.text.trim());
          await _loadVPNProfiles();
          _setAction('VPN profile added');
        }, child: const Text('Save', style: TextStyle(fontSize: 18))),
      ],
    ));
  }

  Color _layerColor(String layer) {
    if (_status['layers'] is! List) return Colors.grey;
    for (final l in _status['layers'] as List) {
      if ((l as Map<String, dynamic>)['layer'] == layer) {
        return (l['healthy'] == true) ? Colors.green : Colors.red;
      }
    }
    return Colors.grey;
  }

  String _layerMsg(String layer) {
    if (_status['layers'] is! List) return '';
    for (final l in _status['layers'] as List) {
      if ((l as Map<String, dynamic>)['layer'] == layer) {
        return l['message'] as String? ?? '';
      }
    }
    return '';
  }

  @override
  Widget build(BuildContext context) {
    final chainState = _chainState['state'] as String? ?? 'down';
    final overall = _status['overall_healthy'] as bool? ?? false;
    final chainUp = chainState == 'up';

    return Scaffold(
      backgroundColor: Colors.black,
      appBar: AppBar(
        backgroundColor: Colors.grey[900],
        title: Row(children: [
          Icon(Icons.shield, color: overall ? Colors.green : Colors.orange, size: 28),
          const SizedBox(width: 8),
          const Text('ParanoidX', style: TextStyle(fontSize: 22, fontWeight: FontWeight.bold)),
        ]),
        actions: [
          IconButton(
            icon: const Icon(Icons.refresh, size: 24),
            onPressed: _refreshAll,
          ),
        ],
      ),
      body: SingleChildScrollView(
        padding: const EdgeInsets.all(12),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [

          // Action message
          if (_actionMsg.isNotEmpty)
            Container(
              width: double.infinity,
              padding: const EdgeInsets.all(8),
              margin: const EdgeInsets.only(bottom: 8),
              decoration: BoxDecoration(color: Colors.grey.shade800, borderRadius: BorderRadius.circular(6)),
              child: Text(_actionMsg, style: const TextStyle(fontSize: 14, color: Colors.cyan), textAlign: TextAlign.center),
            ),

          // Chain state + overall health
          _sectionHeader('Chain State'),
          Row(children: [
            _chainDot(chainUp ? Colors.green : Colors.grey, chainUp ? 'UP' : chainState.toUpperCase()),
            const SizedBox(width: 12),
            Text('Overall: ${overall ? "SECURE" : "INSECURE"}',
              style: TextStyle(fontSize: 18, fontWeight: FontWeight.bold,
                color: overall ? Colors.green : Colors.red)),
            const Spacer(),
            if (overall)
              const Icon(Icons.verified_user, color: Colors.green, size: 28)
            else
              const Icon(Icons.warning, color: Colors.orange, size: 28),
          ]),

          const SizedBox(height: 12),

          // Chain build/teardown buttons
          Row(children: [
            Expanded(child: ElevatedButton.icon(
              onPressed: chainUp ? null : _buildChain,
              icon: const Icon(Icons.build, size: 20),
              label: const Text('Build Chain', style: TextStyle(fontSize: 16)),
              style: ElevatedButton.styleFrom(backgroundColor: Colors.teal, foregroundColor: Colors.white, padding: const EdgeInsets.symmetric(vertical: 12)),
            )),
            const SizedBox(width: 8),
            Expanded(child: ElevatedButton.icon(
              onPressed: chainUp ? _teardownChain : null,
              icon: const Icon(Icons.power_settings_new, size: 20),
              label: const Text('Teardown', style: TextStyle(fontSize: 16)),
              style: ElevatedButton.styleFrom(backgroundColor: Colors.red.shade800, foregroundColor: Colors.white, padding: const EdgeInsets.symmetric(vertical: 12)),
            )),
          ]),

          const SizedBox(height: 16),

          // Layer status
          _sectionHeader('Layer Status'),
          ...['v2ray', 'vpn', 'tor', 'simplex'].map((layer) => Container(
            margin: const EdgeInsets.symmetric(vertical: 3),
            padding: const EdgeInsets.all(10),
            decoration: BoxDecoration(
              color: _layerColor(layer) == Colors.green ? Colors.green.shade900.withValues(alpha: 0.3) : Colors.red.shade900.withValues(alpha: 0.3),
              borderRadius: BorderRadius.circular(8),
              border: Border.all(color: _layerColor(layer).withValues(alpha: 0.5)),
            ),
            child: Row(children: [
              Icon(Icons.circle, size: 14, color: _layerColor(layer)),
              const SizedBox(width: 10),
              Text(layer.toUpperCase(), style: TextStyle(fontSize: 16, fontWeight: FontWeight.bold,
                color: _layerColor(layer) == Colors.green ? Colors.green : Colors.white70)),
              const Spacer(),
              Text(_layerMsg(layer), style: TextStyle(fontSize: 13, color: Colors.grey[400])),
            ]),
          )),

          const SizedBox(height: 16),

          // Layer toggles
          _sectionHeader('Layer Controls'),
          Container(
            padding: const EdgeInsets.all(12),
            decoration: BoxDecoration(color: Colors.grey.shade900, borderRadius: BorderRadius.circular(8)),
            child: Column(children: [
              _toggleRow('V2Ray', _v2rayEnabled, (v) => setState(() => _v2rayEnabled = v), Icons.swap_horiz),
              _toggleRow('VPN', _vpnEnabled, (v) => setState(() => _vpnEnabled = v), Icons.vpn_lock),
              _toggleRow('Tor', _torEnabled, (v) => setState(() => _torEnabled = v), Icons.shield),
              const SizedBox(height: 8),
              SizedBox(width: double.infinity, child: ElevatedButton.icon(
                onPressed: _saveConfig,
                icon: const Icon(Icons.save, size: 20),
                label: const Text('Save Settings', style: TextStyle(fontSize: 16)),
                style: ElevatedButton.styleFrom(backgroundColor: Colors.cyan.shade800, foregroundColor: Colors.white, padding: const EdgeInsets.symmetric(vertical: 12)),
              )),
            ]),
          ),

          const SizedBox(height: 16),

          // End-to-end test
          _sectionHeader('Connectivity Test'),
          Row(children: [
            Expanded(child: ElevatedButton.icon(
              onPressed: _runTest,
              icon: const Icon(Icons.wifi_tethering, size: 20),
              label: const Text('Run Test', style: TextStyle(fontSize: 16)),
              style: ElevatedButton.styleFrom(backgroundColor: Colors.indigo, foregroundColor: Colors.white, padding: const EdgeInsets.symmetric(vertical: 12)),
            )),
          ]),
          if (_testResults.isNotEmpty && _testResults['results'] is Map) ...[
            const SizedBox(height: 8),
            ...(_testResults['results'] as Map<String, dynamic>).entries.map((e) => Padding(
              padding: const EdgeInsets.symmetric(vertical: 2),
              child: Row(children: [
                Icon(e.value == true ? Icons.check_circle : Icons.cancel, size: 16,
                  color: e.value == true ? Colors.green : Colors.red),
                const SizedBox(width: 8),
                Text(e.key, style: const TextStyle(fontSize: 14, color: Colors.white70)),
              ]),
            )),
          ],

          const SizedBox(height: 16),

          // VPN Profiles
          _sectionHeader('VPN Profiles'),
          Row(children: [
            Expanded(child: ElevatedButton.icon(
              onPressed: _showVPNAddDialog,
              icon: const Icon(Icons.add, size: 20),
              label: const Text('Add Profile', style: TextStyle(fontSize: 16)),
              style: ElevatedButton.styleFrom(backgroundColor: Colors.blueGrey, foregroundColor: Colors.white, padding: const EdgeInsets.symmetric(vertical: 10)),
            )),
          ]),
          const SizedBox(height: 6),
          if (_vpnProfiles.isEmpty)
            Container(
              padding: const EdgeInsets.all(12),
              decoration: BoxDecoration(color: Colors.grey.shade900, borderRadius: BorderRadius.circular(6)),
              child: const Text('No VPN profiles configured. Add a WireGuard profile above.', style: TextStyle(fontSize: 14, color: Colors.grey)),
            )
          else
            ..._vpnProfiles.map((p) {
              final prof = p as Map<String, dynamic>;
              final name = prof['name'] as String? ?? '';
              final active = name == _activeProfile;
              return Container(
                margin: const EdgeInsets.symmetric(vertical: 3),
                padding: const EdgeInsets.all(10),
                decoration: BoxDecoration(
                  color: active ? Colors.teal.shade900.withValues(alpha: 0.4) : Colors.grey.shade900,
                  borderRadius: BorderRadius.circular(6),
                  border: active ? Border.all(color: Colors.teal) : null,
                ),
                child: Row(children: [
                  Icon(active ? Icons.vpn_lock : Icons.vpn_key, size: 20, color: active ? Colors.teal : Colors.grey),
                  const SizedBox(width: 8),
                  Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                    Text(name, style: const TextStyle(fontSize: 16, color: Colors.white)),
                    if (prof['description'] != null && (prof['description'] as String).isNotEmpty)
                      Text(prof['description'] as String, style: TextStyle(fontSize: 13, color: Colors.grey[500])),
                  ])),
                  if (!active)
                    IconButton(
                      icon: const Icon(Icons.play_arrow, color: Colors.teal, size: 22),
                      onPressed: () async {
                        await widget.client.paranoidx.vpnUp(name);
                        await _loadVPNProfiles();
                        _setAction('VPN $name starting');
                      },
                    )
                  else
                    IconButton(
                      icon: const Icon(Icons.stop, color: Colors.red, size: 22),
                      onPressed: () async {
                        await widget.client.paranoidx.vpnDown(name);
                        await _loadVPNProfiles();
                        _setAction('VPN $name stopping');
                      },
                    ),
                  IconButton(
                    icon: const Icon(Icons.delete_outline, color: Colors.red, size: 20),
                    onPressed: () async {
                      await widget.client.paranoidx.vpnDelete(name);
                      await _loadVPNProfiles();
                      _setAction('VPN profile $name deleted');
                    },
                  ),
                ]),
              );
            }),

          const SizedBox(height: 16),

          // Config
          _sectionHeader('Configuration'),
          Container(
            width: double.infinity,
            padding: const EdgeInsets.all(10),
            decoration: BoxDecoration(color: Colors.grey.shade900, borderRadius: BorderRadius.circular(6)),
            child: Text(
              _config.isEmpty ? 'No config data' : JsonEncoder.withIndent('  ').convert(_config),
              style: const TextStyle(fontSize: 12, color: Colors.white70, fontFamily: 'monospace'),
            ),
          ),

          const SizedBox(height: 24),

          // Emergency kill switch
          Center(child: SizedBox(
            width: double.infinity,
            child: ElevatedButton.icon(
              onPressed: () async {
                final confirm = await showDialog<bool>(
                  context: context,
                  builder: (ctx) => AlertDialog(
                    backgroundColor: Colors.grey[900],
                    title: const Text('⚠ EMERGENCY', style: TextStyle(fontSize: 22, color: Colors.red)),
                    content: const Text('This will tear down ALL proxy layers immediately. Continue?', style: TextStyle(fontSize: 16)),
                    actions: [
                      TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('Cancel', style: TextStyle(fontSize: 18))),
                      ElevatedButton(
                        onPressed: () => Navigator.pop(ctx, true),
                        style: ElevatedButton.styleFrom(backgroundColor: Colors.red),
                        child: const Text('KILL ALL', style: TextStyle(fontSize: 18, color: Colors.white)),
                      ),
                    ],
                  ),
                );
                if (confirm == true) {
                  await _teardownChain();
                  _setAction('⚠ ALL PROXY LAYERS TERMINATED');
                }
              },
              icon: const Icon(Icons.report, size: 24),
              label: const Text('EMERGENCY KILL SWITCH', style: TextStyle(fontSize: 16, fontWeight: FontWeight.bold)),
              style: ElevatedButton.styleFrom(
                backgroundColor: Colors.red.shade900,
                foregroundColor: Colors.white,
                padding: const EdgeInsets.symmetric(vertical: 16),
              ),
            ),
          )),

          const SizedBox(height: 24),
        ]),
      ),
    );
  }

  Widget _sectionHeader(String text) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 6, top: 4),
      child: Text(text, style: TextStyle(fontSize: 18, fontWeight: FontWeight.bold, color: Colors.grey[300])),
    );
  }

  Widget _toggleRow(String label, bool value, ValueChanged<bool> onChanged, IconData icon) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 4),
      child: Row(children: [
        Icon(icon, size: 22, color: value ? Colors.cyan : Colors.grey),
        const SizedBox(width: 10),
        Text(label, style: TextStyle(fontSize: 16, color: Colors.white, fontWeight: FontWeight.w500)),
        const Spacer(),
        Switch(
          value: value,
          onChanged: onChanged,
          activeColor: Colors.cyan,
          activeTrackColor: Colors.cyan.withValues(alpha: 0.4),
        ),
      ]),
    );
  }

  Widget _chainDot(Color color, String label) {
    return Container(
      width: 48,
      height: 48,
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.2),
        shape: BoxShape.circle,
        border: Border.all(color: color, width: 2),
      ),
      child: Center(child: Text(label, style: TextStyle(fontSize: 10, fontWeight: FontWeight.bold, color: color))),
    );
  }
}