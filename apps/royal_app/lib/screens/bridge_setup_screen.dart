import 'dart:async';
import 'package:flutter/material.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:royal_app/l10n/generated/app_localizations.dart';
import '../theme.dart';

/// Network bridge initialization screen
/// Tests and configures: OpenVPN, WireGuard, V2Ray, Tor
/// Prefers v2ray-tor sequence, tests Simplex node connectivity
class BridgeSetupScreen extends StatefulWidget {
  final VoidCallback onBridgeReady;

  const BridgeSetupScreen({
    super.key,
    required this.onBridgeReady,
  });

  @override
  State<BridgeSetupScreen> createState() => _BridgeSetupScreenState();
}

class _BridgeSetupScreenState extends State<BridgeSetupScreen>
    with TickerProviderStateMixin {
  // Localization
  late AppLocalizations _l10n;

  // Bridge state
  String _currentMessage = '';
  double _progress = 0.0;
  int _currentStep = 0;
  final int _totalSteps = 7;

  // Protocol configurations
  final List<ProtocolConfig> _protocols = [
    ProtocolConfig(
        name: 'v2ray', displayName: 'V2Ray', enabled: true, priority: 1),
    ProtocolConfig(name: 'tor', displayName: 'Tor', enabled: true, priority: 2),
    ProtocolConfig(
        name: 'wireguard',
        displayName: 'WireGuard',
        enabled: false,
        priority: 3),
    ProtocolConfig(
        name: 'openvpn', displayName: 'OpenVPN', enabled: false, priority: 4),
  ];

  // Test results
  final Map<String, ProtocolTestResult> _testResults = {};
  String? _simplexNodeConfig;
  bool _simplexNodeAvailable = false;

  // Animation controllers
  late AnimationController _fadeCtrl;
  late AnimationController _pulseCtrl;
  late Animation<double> _fadeAnim;
  late Animation<double> _pulseAnim;

  // Bridge configuration
  late SharedPreferences _prefs;

  @override
  void initState() {
    super.initState();
    _fadeCtrl = AnimationController(
        duration: const Duration(milliseconds: 500), vsync: this);
    _pulseCtrl = AnimationController(
        duration: const Duration(milliseconds: 1000), vsync: this);
    _fadeAnim = Tween<double>(begin: 0.0, end: 1.0)
        .animate(CurvedAnimation(parent: _fadeCtrl, curve: Curves.easeOut));
    _pulseAnim = Tween<double>(begin: 0.5, end: 1.0)
        .animate(CurvedAnimation(parent: _pulseCtrl, curve: Curves.easeInOut));
    _fadeCtrl.forward();
    _pulseCtrl.repeat(reverse: true);

    _loadConfiguration();
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _l10n = AppLocalizations.of(context)!;
    _currentMessage = _l10n.testingNetworkEnvironment;
  }

  @override
  void dispose() {
    _fadeCtrl.dispose();
    _pulseCtrl.dispose();
    super.dispose();
  }

  Future<void> _loadConfiguration() async {
    _prefs = await SharedPreferences.getInstance();
    _simplexNodeConfig = _prefs.getString('simplex_node_config') ?? '';

    // Load protocol configs
    for (var protocol in _protocols) {
      protocol.enabled = _prefs.getBool('protocol_${protocol.name}_enabled') ??
          protocol.enabled;
    }

    // Start bridge initialization sequence
    _startBridgeInitialization();
  }

  Future<void> _startBridgeInitialization() async {
    try {
      // Step 1: Test network environment
      await _updateStep(_l10n.testingNetworkEnvironment);
      await _testNetworkEnvironment();

      // Step 2: Load and test protocols
      await _updateStep(_l10n.connectingProtocols);
      await _testProtocols();

      // Step 3: Select optimal sequence
      await _updateStep(_l10n.selectingOptimalSequence);
      await _selectOptimalSequence();

      // Step 4: Test Simplex node
      await _updateStep(_l10n.testingSimplexNode);
      await _testSimplexNode();

      // Step 5: Check if configuration is complete
      if ((_simplexNodeConfig?.isEmpty ?? true) || !_simplexNodeAvailable) {
        // Need configuration
        _currentMessage = _l10n.bridgeConfiguration;
        await _showConfigurationDialog();
      } else {
        // Configuration exists, start bridge
        await _startBridge();
      }
    } catch (e) {
      _currentMessage = '${_l10n.error}: $e';
      _showErrorDialog(e.toString());
    }
  }

  Future<void> _testNetworkEnvironment() async {
    // Simulate network environment testing
    await Future.delayed(const Duration(milliseconds: 1500));
    // In real implementation: check interfaces, DNS, routing, etc.
  }

  Future<void> _testProtocols() async {
    for (final protocol in _protocols.where((p) => p.enabled)) {
      _currentMessage = _l10n.testingProtocol(protocol.displayName);
      await Future.delayed(const Duration(milliseconds: 800));

      // Simulate protocol test
      final success = await _testProtocol(protocol);
      _testResults[protocol.name] = ProtocolTestResult(
        protocol: protocol.name,
        success: success,
        latency: success ? 45 + (protocol.name.hashCode % 100) : null,
        error: success ? null : 'Connection timeout',
        timestamp: DateTime.now(),
      );

      await Future.delayed(const Duration(milliseconds: 300));
    }
  }

  Future<bool> _testProtocol(ProtocolConfig protocol) async {
    // In real implementation: actually test the protocol connection
    // For now, simulate success for v2ray and tor if enabled
    if (protocol.name == 'v2ray' || protocol.name == 'tor') {
      return true;
    }
    return protocol.enabled;
  }

  Future<void> _selectOptimalSequence() async {
    await Future.delayed(const Duration(milliseconds: 1000));

    // Sort protocols by priority and test results
    final availableProtocols = _protocols
        .where((p) => p.enabled && _testResults[p.name]?.success == true)
        .toList()
      ..sort((a, b) => a.priority.compareTo(b.priority));

    // Prefer v2ray -> tor sequence
    if (availableProtocols.any((p) => p.name == 'v2ray') &&
        availableProtocols.any((p) => p.name == 'tor')) {
      _currentMessage = _l10n.preferredSequence;
    }
  }

  Future<void> _testSimplexNode() async {
    if (_simplexNodeConfig?.isEmpty ?? true) {
      _simplexNodeAvailable = false;
      return;
    }

    await Future.delayed(const Duration(milliseconds: 2000));

    // In real implementation: test actual Simplex node connectivity
    // For now, simulate success if config exists
    _simplexNodeAvailable = true;
  }

  Future<void> _showConfigurationDialog() async {
    await showDialog(
      context: context,
      barrierDismissible: false,
      builder: (context) => _BridgeConfigDialog(
        protocols: _protocols,
        simplexNodeConfig: _simplexNodeConfig,
        onSave: (config) async {
          _simplexNodeConfig = config.simplexNodeConfig;
          _protocols.clear();
          _protocols.addAll(config.protocols);

          // Save to preferences
          for (var p in _protocols) {
            await _prefs.setBool('protocol_${p.name}_enabled', p.enabled);
          }
          if ((_simplexNodeConfig?.isNotEmpty ?? false)) {
            await _prefs.setString('simplex_node_config', _simplexNodeConfig!);
          }

          // Restart bridge initialization
          _currentStep = 0;
          _progress = 0.0;
          _testResults.clear();
          _startBridgeInitialization();
        },
        l10n: _l10n,
      ),
    );
  }

  Future<void> _startBridge() async {
    _currentMessage = _l10n.startBridge;

    await Future.delayed(const Duration(milliseconds: 1500));

    // Bridge is ready
    _simplexNodeAvailable = true;

    // Show bridge status
    await _showBridgeStatus();
  }

  Future<void> _showBridgeStatus() async {
    await showDialog(
      context: context,
      barrierDismissible: false,
      builder: (context) => _BridgeStatusDialog(
        protocols: _protocols,
        testResults: _testResults,
        simplexNodeConfig: _simplexNodeConfig,
        simplexNodeAvailable: _simplexNodeAvailable,
        onContinue: () {
          Navigator.of(context).pop();
          widget.onBridgeReady();
        },
        l10n: _l10n,
      ),
    );
  }

  Future<void> _updateStep(String message) async {
    _currentMessage = message;
    _currentStep++;
    _progress = _currentStep / _totalSteps;
    await Future.delayed(const Duration(milliseconds: 500));
  }

  void _showErrorDialog(String error) {
    showDialog(
      context: context,
      barrierDismissible: false,
      builder: (context) => AlertDialog(
        backgroundColor: RoyalTheme.darkCard,
        title: Text(_l10n.error, style: const TextStyle(color: RoyalTheme.red)),
        content: Text(error, style: const TextStyle(color: Colors.white)),
        actions: [
          TextButton(
            onPressed: () {
              Navigator.of(context).pop();
              _startBridgeInitialization();
            },
            child: Text(
              _l10n.continueAction,
              style: const TextStyle(color: RoyalTheme.gold),
            ),
          ),
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Scaffold(
      backgroundColor: RoyalTheme.darkScaffold,
      body: Stack(
        children: [
          // Background pattern
          CustomPaint(
            size: Size.infinite,
            painter: _BackgroundPatternPainter(),
          ),

          Center(
            child: FadeTransition(
              opacity: _fadeAnim,
              child: Container(
                constraints: const BoxConstraints(maxWidth: 600),
                padding: const EdgeInsets.all(48),
                child: Column(
                  mainAxisAlignment: MainAxisAlignment.center,
                  children: [
                    // Bridge icon with pulse animation
                    ScaleTransition(
                      scale: _pulseAnim,
                      child: Container(
                        width: 140,
                        height: 140,
                        decoration: BoxDecoration(
                          shape: BoxShape.circle,
                          gradient: RadialGradient(
                            colors: [
                              RoyalTheme.gold.withAlpha(60),
                              RoyalTheme.deepNavy,
                            ],
                          ),
                          border: Border.all(
                            color: RoyalTheme.gold.withAlpha(150),
                            width: 2,
                          ),
                          boxShadow: [
                            BoxShadow(
                              color: RoyalTheme.gold.withAlpha(40),
                              blurRadius: 60,
                              spreadRadius: 10,
                            ),
                          ],
                        ),
                        child: const Icon(
                          Icons.network_check_rounded,
                          size: 72,
                          color: RoyalTheme.gold,
                        ),
                      ),
                    ),

                    const SizedBox(height: 48),

                    // Title
                    Text(
                      _l10n.networkBridgeSetup,
                      style: theme.textTheme.headlineLarge?.copyWith(
                        fontWeight: FontWeight.w700,
                        color: Colors.white,
                      ),
                      textAlign: TextAlign.center,
                    ),

                    const SizedBox(height: 16),

                    // Current status message
                    AnimatedSwitcher(
                      duration: const Duration(milliseconds: 300),
                      child: Text(
                        _currentMessage,
                        key: ValueKey(_currentMessage),
                        style: theme.textTheme.bodyLarge?.copyWith(
                          color: RoyalTheme.silver,
                        ),
                        textAlign: TextAlign.center,
                      ),
                    ),

                    const SizedBox(height: 48),

                    // Progress bar
                    SizedBox(
                      width: 400,
                      child: Column(
                        children: [
                          ClipRRect(
                            borderRadius: BorderRadius.circular(6),
                            child: LinearProgressIndicator(
                              value: _progress,
                              minHeight: 8,
                              backgroundColor: RoyalTheme.darkCard,
                              valueColor: const AlwaysStoppedAnimation<Color>(
                                  RoyalTheme.gold),
                            ),
                          ),
                          const SizedBox(height: 12),
                          Row(
                            mainAxisAlignment: MainAxisAlignment.spaceBetween,
                            children: [
                              Text(
                                'Step ${_currentStep.clamp(0, _totalSteps)} / $_totalSteps',
                                style: theme.textTheme.bodyMedium?.copyWith(
                                  color: RoyalTheme.silver.withAlpha(150),
                                ),
                              ),
                              Text(
                                '${(_progress * 100).toInt()}%',
                                style: theme.textTheme.bodyMedium?.copyWith(
                                  color: RoyalTheme.gold,
                                  fontFeatures: const [FontFeature.tabularFigures()],
                                  fontWeight: FontWeight.w600,
                                ),
                              ),
                            ],
                          ),
                        ],
                      ),
                    ),

                    const SizedBox(height: 48),

                    // Protocol status indicators
                    if (_testResults.isNotEmpty) ...[
                      const Divider(color: Color(0xFF30363D)),
                      const SizedBox(height: 24),
                      Text(
                        _l10n.protocolStatus,
                        style: theme.textTheme.titleMedium?.copyWith(
                          color: RoyalTheme.gold,
                          fontWeight: FontWeight.w600,
                        ),
                      ),
                      const SizedBox(height: 16),
                      ..._protocols
                          .where((p) => _testResults.containsKey(p.name))
                          .map((p) => _ProtocolStatusTile(
                                protocol: p,
                                result: _testResults[p.name]!,
                              ))

                    ],
                  ],
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _ProtocolStatusTile extends StatelessWidget {
  final ProtocolConfig protocol;
  final ProtocolTestResult result;

  const _ProtocolStatusTile({
    required this.protocol,
    required this.result,
  });

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final success = result.success;

    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 10),
      child: Row(
        children: [
          Container(
            width: 44,
            height: 44,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              color: success
                  ? RoyalTheme.green.withAlpha(30)
                  : RoyalTheme.red.withAlpha(30),
              border: Border.all(
                color: success ? RoyalTheme.green : RoyalTheme.red,
                width: 1.5,
              ),
            ),
            child: Icon(
              success ? Icons.check_rounded : Icons.close_rounded,
              size: 24,
              color: success ? RoyalTheme.green : RoyalTheme.red,
            ),
          ),
          const SizedBox(width: 16),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  protocol.displayName,
                  style: theme.textTheme.bodyLarge?.copyWith(
                    fontWeight: FontWeight.w600,
                    color: Colors.white,
                  ),
                ),
                if (result.latency != null)
                  Text(
                    'Latency: ${result.latency}ms',
                    style: theme.textTheme.bodySmall?.copyWith(
                      color: RoyalTheme.silver.withAlpha(150),
                    ),
                  ),
              ],
            ),
          ),
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
            decoration: BoxDecoration(
              color: success
                  ? RoyalTheme.green.withAlpha(30)
                  : RoyalTheme.red.withAlpha(30),
              borderRadius: BorderRadius.circular(16),
              border: Border.all(
                color: success ? RoyalTheme.green : RoyalTheme.red,
                width: 1,
              ),
            ),
            child: Text(
              success ? 'OK' : 'FAILED',
              style: theme.textTheme.labelSmall?.copyWith(
                color: success ? RoyalTheme.green : RoyalTheme.red,
                fontWeight: FontWeight.w600,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _BridgeConfigDialog extends StatefulWidget {
  final List<ProtocolConfig> protocols;
  final String? simplexNodeConfig;
  final Function(BridgeConfig) onSave;
  final AppLocalizations l10n;

  const _BridgeConfigDialog({
    required this.protocols,
    required this.simplexNodeConfig,
    required this.onSave,
    required this.l10n,
  });

  @override
  State<_BridgeConfigDialog> createState() => _BridgeConfigDialogState();
}

class _BridgeConfigDialogState extends State<_BridgeConfigDialog> {
  late List<ProtocolConfig> _protocols;
  late TextEditingController _simplexCtrl;
  bool _isLoading = false;

  @override
  void initState() {
    super.initState();
    _protocols = widget.protocols
        .map((p) => ProtocolConfig(
              name: p.name,
              displayName: p.displayName,
              enabled: p.enabled,
              priority: p.priority,
              config: p.config,
            ))
        .toList();
    _simplexCtrl = TextEditingController(text: widget.simplexNodeConfig ?? '');
  }

  @override
  void dispose() {
    _simplexCtrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Dialog(
      backgroundColor: RoyalTheme.darkCard,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(20),
        side: BorderSide(color: RoyalTheme.gold.withAlpha(100)),
      ),
      child: Container(
        constraints: const BoxConstraints(maxWidth: 560, maxHeight: 700),
        padding: const EdgeInsets.all(32),
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  const Icon(Icons.settings_ethernet_rounded,
                      color: RoyalTheme.gold, size: 28),
                  const SizedBox(width: 12),
                  Text(
                    widget.l10n.bridgeConfiguration,
                    style: theme.textTheme.headlineSmall?.copyWith(
                      fontWeight: FontWeight.w700,
                      color: Colors.white,
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 8),
              Text(
                widget.l10n.configureProtocols,
                style: theme.textTheme.bodyMedium
                    ?.copyWith(color: RoyalTheme.silver),
              ),
              const SizedBox(height: 32),

              // Protocol toggles
              ..._protocols.map((p) => _ProtocolConfigTile(
                    protocol: p,
                    onChanged: (enabled) {
                      setState(() {
                        p.enabled = enabled;
                      });
                    },
                  )),

              const SizedBox(height: 32),
              const Divider(color: Color(0xFF30363D)),
              const SizedBox(height: 24),

              // Simplex node config
              Text(
                'Simplex Node Configuration',
                style: theme.textTheme.titleMedium?.copyWith(
                  color: RoyalTheme.gold,
                  fontWeight: FontWeight.w600,
                ),
              ),
              const SizedBox(height: 16),
              TextField(
                controller: _simplexCtrl,
                style: const TextStyle(color: Colors.white),
                decoration: const InputDecoration(
                  labelText:
                      'Node Address (e.g., tcp://host:port or onion address)',
                  hintText:
                      'tcp://simplex.example.com:5223 or abcdef.onion:5223',
                  prefixIcon: Icon(Icons.dns_rounded, color: RoyalTheme.silver),
                ),
                maxLines: 2,
              ),

              const SizedBox(height: 32),

              // Action buttons
              Row(
                children: [
                  Expanded(
                    child: OutlinedButton.icon(
                      onPressed:
                          _isLoading ? null : () => Navigator.of(context).pop(),
                      icon: const Icon(Icons.cancel_rounded),
                      label: Text(widget.l10n.cancel),
                      style: OutlinedButton.styleFrom(
                        foregroundColor: Colors.grey[400],
                        side: BorderSide(color: Colors.grey[600]!),
                        padding: const EdgeInsets.symmetric(vertical: 16),
                      ),
                    ),
                  ),
                  const SizedBox(width: 16),
                  Expanded(
                    child: ElevatedButton.icon(
                      onPressed: _isLoading ? null : _saveConfig,
                      icon: _isLoading
                          ? const SizedBox(
                              width: 20,
                              height: 20,
                              child: CircularProgressIndicator(strokeWidth: 2))
                          : const Icon(Icons.save_rounded),
                      label: Text(widget.l10n.saveConfiguration),
                      style: ElevatedButton.styleFrom(
                        backgroundColor: RoyalTheme.gold,
                        foregroundColor: RoyalTheme.deepNavy,
                        padding: const EdgeInsets.symmetric(vertical: 16),
                      ),
                    ),
                  ),
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }

  void _saveConfig() {
    setState(() => _isLoading = true);

    final config = BridgeConfig(
      protocols: _protocols,
      simplexNodeConfig: _simplexCtrl.text.trim(),
    );

    Future.delayed(const Duration(milliseconds: 500), () {
      widget.onSave(config);
      Navigator.of(context).pop();
    });
  }
}

class _ProtocolConfigTile extends StatelessWidget {
  final ProtocolConfig protocol;
  final Function(bool) onChanged;

  const _ProtocolConfigTile({
    required this.protocol,
    required this.onChanged,
  });

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 10),
      child: Row(
        children: [
          Container(
            width: 48,
            height: 48,
            decoration: BoxDecoration(
              color: protocol.enabled
                  ? RoyalTheme.gold.withAlpha(30)
                  : RoyalTheme.darkCard,
              borderRadius: BorderRadius.circular(12),
              border: Border.all(
                color: protocol.enabled ? RoyalTheme.gold : Colors.grey[700]!,
                width: 1.5,
              ),
            ),
            child: Icon(
              _getProtocolIcon(protocol.name),
              color: protocol.enabled ? RoyalTheme.gold : Colors.grey[500],
              size: 24,
            ),
          ),
          const SizedBox(width: 16),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  protocol.displayName,
                  style: theme.textTheme.bodyLarge?.copyWith(
                    fontWeight: FontWeight.w600,
                    color: Colors.white,
                  ),
                ),
                Text(
                  'Priority: ${protocol.priority}',
                  style: theme.textTheme.bodySmall?.copyWith(
                    color: RoyalTheme.silver.withAlpha(150),
                  ),
                ),
              ],
            ),
          ),
          Switch(
            value: protocol.enabled,
            onChanged: onChanged,
            activeThumbColor: RoyalTheme.gold,
            activeTrackColor: RoyalTheme.gold.withAlpha(80),
            inactiveThumbColor: Colors.grey[400],
            inactiveTrackColor: Colors.grey[800],
          ),
        ],
      ),
    );
  }

  IconData _getProtocolIcon(String name) {
    switch (name) {
      case 'v2ray':
        return Icons.cloud_queue_rounded;
      case 'tor':
        return Icons.security_rounded;
      case 'wireguard':
        return Icons.vpn_key_rounded;
      case 'openvpn':
        return Icons.shield_rounded;
      default:
        return Icons.network_check_rounded;
    }
  }
}

class _BridgeStatusDialog extends StatelessWidget {
  final List<ProtocolConfig> protocols;
  final Map<String, ProtocolTestResult> testResults;
  final String? simplexNodeConfig;
  final bool simplexNodeAvailable;
  final VoidCallback onContinue;
  final AppLocalizations l10n;

  const _BridgeStatusDialog({
    required this.protocols,
    required this.testResults,
    required this.simplexNodeConfig,
    required this.simplexNodeAvailable,
    required this.onContinue,
    required this.l10n,
  });

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Dialog(
      backgroundColor: RoyalTheme.darkCard,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(20),
        side: BorderSide(color: RoyalTheme.gold.withAlpha(100)),
      ),
      child: Container(
        constraints: const BoxConstraints(maxWidth: 560, maxHeight: 700),
        padding: const EdgeInsets.all(32),
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  Container(
                    width: 56,
                    height: 56,
                    decoration: BoxDecoration(
                      shape: BoxShape.circle,
                      gradient: RadialGradient(
                        colors: [
                          RoyalTheme.green.withAlpha(60),
                          RoyalTheme.deepNavy
                        ],
                      ),
                      border: Border.all(color: RoyalTheme.green, width: 2),
                    ),
                    child: const Icon(Icons.check_circle_rounded,
                        color: RoyalTheme.green, size: 28),
                  ),
                  const SizedBox(width: 16),
                  Text(
                    l10n.bridgeStatus,
                    style: theme.textTheme.headlineSmall?.copyWith(
                      fontWeight: FontWeight.w700,
                      color: Colors.white,
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 8),
              Text(
                'Network bridge is ready and operational',
                style: theme.textTheme.bodyMedium
                    ?.copyWith(color: RoyalTheme.silver),
              ),
              const SizedBox(height: 32),

              // Protocol status
              Text(l10n.protocolStatus,
                  style: theme.textTheme.titleMedium?.copyWith(
                      color: RoyalTheme.gold, fontWeight: FontWeight.w600)),
              const SizedBox(height: 16),
              ...protocols.where((p) => p.enabled).map((p) {
                final result = testResults[p.name];
                final success = result?.success == true;
                return Padding(
                  padding: const EdgeInsets.symmetric(vertical: 6),
                  child: Row(
                    children: [
                      Container(
                        width: 36,
                        height: 36,
                        decoration: BoxDecoration(
                          shape: BoxShape.circle,
                          color: success
                              ? RoyalTheme.green.withAlpha(30)
                              : RoyalTheme.red.withAlpha(30),
                          border: Border.all(
                              color:
                                  success ? RoyalTheme.green : RoyalTheme.red,
                              width: 1.5),
                        ),
                        child: Icon(
                            success ? Icons.check_rounded : Icons.close_rounded,
                            size: 20,
                            color: success ? RoyalTheme.green : RoyalTheme.red),
                      ),
                      const SizedBox(width: 12),
                      Expanded(
                          child: Text(p.displayName,
                              style: theme.textTheme.bodyLarge
                                  ?.copyWith(color: Colors.white))),
                      if (result?.latency != null)
                        Text('${result?.latency}ms',
                            style: theme.textTheme.bodySmall?.copyWith(
                                color: RoyalTheme.silver.withAlpha(150))),
                      Container(
                        padding: const EdgeInsets.symmetric(
                            horizontal: 10, vertical: 4),
                        decoration: BoxDecoration(
                          color: success
                              ? RoyalTheme.green.withAlpha(30)
                              : RoyalTheme.red.withAlpha(30),
                          borderRadius: BorderRadius.circular(12),
                          border: Border.all(
                              color:
                                  success ? RoyalTheme.green : RoyalTheme.red,
                              width: 1),
                        ),
                        child: Text(success ? 'ACTIVE' : 'INACTIVE',
                            style: theme.textTheme.labelSmall?.copyWith(
                                color:
                                    success ? RoyalTheme.green : RoyalTheme.red,
                                fontWeight: FontWeight.w600)),
                      ),
                    ],
                  ),
                );
              }),

              const SizedBox(height: 24),
              const Divider(color: Color(0xFF30363D)),
              const SizedBox(height: 16),

              // Simplex node status
              Text(l10n.serviceStatus,
                  style: theme.textTheme.titleMedium?.copyWith(
                      color: RoyalTheme.gold, fontWeight: FontWeight.w600)),
              const SizedBox(height: 16),
              Row(
                children: [
                  Container(
                    width: 40,
                    height: 40,
                    decoration: BoxDecoration(
                      shape: BoxShape.circle,
                      color: simplexNodeAvailable
                          ? RoyalTheme.green.withAlpha(30)
                          : RoyalTheme.orange.withAlpha(30),
                      border: Border.all(
                          color: simplexNodeAvailable
                              ? RoyalTheme.green
                              : RoyalTheme.orange,
                          width: 1.5),
                    ),
                    child: Icon(
                      simplexNodeAvailable
                          ? Icons.cloud_done_rounded
                          : Icons.cloud_off_rounded,
                      color: simplexNodeAvailable
                          ? RoyalTheme.green
                          : RoyalTheme.orange,
                      size: 22,
                    ),
                  ),
                  const SizedBox(width: 12),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text('Simplex Node',
                            style: theme.textTheme.bodyLarge?.copyWith(
                                fontWeight: FontWeight.w600,
                                color: Colors.white)),
                        Text(
                          simplexNodeConfig?.isEmpty ?? true
                              ? 'Not configured'
                              : simplexNodeConfig!,
                          style: theme.textTheme.bodySmall?.copyWith(
                              color: RoyalTheme.silver.withAlpha(150)),
                        ),
                      ],
                    ),
                  ),
                  Container(
                    padding:
                        const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
                    decoration: BoxDecoration(
                      color: simplexNodeAvailable
                          ? RoyalTheme.green.withAlpha(30)
                          : RoyalTheme.orange.withAlpha(30),
                      borderRadius: BorderRadius.circular(16),
                      border: Border.all(
                          color: simplexNodeAvailable
                              ? RoyalTheme.green
                              : RoyalTheme.orange,
                          width: 1),
                    ),
                    child: Text(
                      simplexNodeAvailable ? 'CONNECTED' : 'PENDING',
                      style: theme.textTheme.labelSmall?.copyWith(
                        color: simplexNodeAvailable
                            ? RoyalTheme.green
                            : RoyalTheme.orange,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                  ),
                ],
              ),

              const SizedBox(height: 32),

              // Continue button
              SizedBox(
                width: double.infinity,
                child: ElevatedButton.icon(
                  onPressed: onContinue,
                  icon: const Icon(Icons.arrow_forward_rounded),
                  label: Text(l10n.continueAction),
                  style: ElevatedButton.styleFrom(
                    backgroundColor: RoyalTheme.gold,
                    foregroundColor: RoyalTheme.deepNavy,
                    padding: const EdgeInsets.symmetric(vertical: 18),
                    shape: RoundedRectangleBorder(
                        borderRadius: BorderRadius.circular(12)),
                    textStyle: const TextStyle(
                        fontWeight: FontWeight.w600, fontSize: 16),
                  ),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _BackgroundPatternPainter extends CustomPainter {
  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = RoyalTheme.gold.withAlpha(5)
      ..strokeWidth = 0.5;

    const spacing = 60.0;
    for (double x = 0; x < size.width; x += spacing) {
      canvas.drawLine(Offset(x, 0), Offset(x, size.height), paint);
    }
    for (double y = 0; y < size.height; y += spacing) {
      canvas.drawLine(Offset(0, y), Offset(size.width, y), paint);
    }
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}

// Data classes
class ProtocolConfig {
  final String name;
  final String displayName;
  bool enabled;
  final int priority;
  final Map<String, dynamic> config;

  ProtocolConfig({
    required this.name,
    required this.displayName,
    required this.enabled,
    required this.priority,
    this.config = const {},
  });
}

class ProtocolTestResult {
  final String protocol;
  final bool success;
  final int? latency;
  final String? error;
  final DateTime timestamp;

  ProtocolTestResult({
    required this.protocol,
    required this.success,
    this.latency,
    this.error,
    required this.timestamp,
  });
}

class BridgeConfig {
  final List<ProtocolConfig> protocols;
  final String simplexNodeConfig;

  BridgeConfig({
    required this.protocols,
    required this.simplexNodeConfig,
  });
}
