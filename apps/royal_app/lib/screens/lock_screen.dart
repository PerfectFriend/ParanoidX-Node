import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:models/models.dart';
import 'package:provider/provider.dart';
import '../main.dart';
import '../theme.dart';

/// Lock screen for Royal App - requires PIN to unlock a specific profile.
/// Supports: multiple profiles, 8-digit PIN, progressive timeout, panic button, seed recovery.
class LockScreen extends StatefulWidget {
  final String profileId;
  final VoidCallback onUnlocked;
  final VoidCallback? onBackToProfiles;
  final VoidCallback? onRecoverFromSeed;
  final String? initialError;

  const LockScreen({
    super.key,
    required this.profileId,
    required this.onUnlocked,
    this.onBackToProfiles,
    this.onRecoverFromSeed,
    this.initialError,
  });

  @override
  State<LockScreen> createState() => _LockScreenState();
}

class _LockScreenState extends State<LockScreen> with TickerProviderStateMixin {
  final _pinCtrl = TextEditingController();
  late AnimationController _shakeCtrl;
  late Animation<double> _shakeAnim;
  bool _obscurePin = true;
  int _pinAttempts = 0;
  bool _lockedOut = false;
  Timer? _lockoutTimer;
  int _remainingLockoutSeconds = 0;
  bool _isUnlocking = false;
  String? _profileName;
  bool _panicMode = false;

  @override
  void initState() {
    super.initState();
    _shakeCtrl = AnimationController(
      duration: const Duration(milliseconds: 400),
      vsync: this,
    );
    _shakeAnim = Tween<double>(begin: 0, end: 10)
        .chain(CurveTween(curve: Curves.elasticIn))
        .animate(_shakeCtrl);
    _loadProfileInfo();
    _checkPanicMode();
    if (widget.initialError != null) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
              content: Text(widget.initialError!),
              backgroundColor: RoyalTheme.red),
        );
      });
    }
  }

  Future<void> _loadProfileInfo() async {
    final prefs = await SharedPreferences.getInstance();
    setState(() {
      _profileName =
          prefs.getString('${widget.profileId}_name') ?? widget.profileId;
    });
  }

  Future<void> _checkPanicMode() async {
    final prefs = await SharedPreferences.getInstance();
    final panicMode = prefs.getBool('${widget.profileId}_panic_mode') ?? false;
    setState(() => _panicMode = panicMode);
  }

  @override
  void dispose() {
    _pinCtrl.dispose();
    _shakeCtrl.dispose();
    _lockoutTimer?.cancel();
    super.dispose();
  }

  int _calculateLockoutDuration(int attempt) {
    // First error: 30s, then doubles: 30, 60, 120, 240, 480...
    if (attempt <= 0) return 0;
    return 30 * (1 << (attempt - 1));
  }

  void _startLockout() {
    final duration = _calculateLockoutDuration(_pinAttempts);
    _remainingLockoutSeconds = duration;
    _lockedOut = true;

    _lockoutTimer?.cancel();
    _lockoutTimer = Timer.periodic(const Duration(seconds: 1), (timer) {
      if (!mounted) {
        timer.cancel();
        return;
      }
      setState(() {
        _remainingLockoutSeconds--;
        if (_remainingLockoutSeconds <= 0) {
          _lockedOut = false;
          _pinAttempts = 0;
          timer.cancel();
        }
      });
    });
  }

  Future<void> _onUnlock() async {
    if (_lockedOut || _isUnlocking) return;

    final pin = _pinCtrl.text;
    if (pin.length != 8 || !RegExp(r'^\d{8}$').hasMatch(pin)) {
      _shake();
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
            content: Text('PIN must be exactly 8 digits'),
            backgroundColor: RoyalTheme.red),
      );
      return;
    }

    debugPrint('UNLOCK: Starting unlock for profile ${widget.profileId}');
    debugPrint('UNLOCK: PIN length: ${pin.length}');

    setState(() => _isUnlocking = true);

    try {
      final appState = context.read<AppState>();
      final prefs = await SharedPreferences.getInstance();
      final identityService = await SecureIdentityService.instance;

      // Get profile-specific encrypted data and PIN hash
      final encrypted = prefs.getString('${widget.profileId}_encrypted');
      final pinHash = prefs.getString('${widget.profileId}_pin_hash');

      debugPrint('UNLOCK: Encrypted data length: ${encrypted?.length ?? 0}');
      debugPrint('UNLOCK: PIN hash length: ${pinHash?.length ?? 0}');

      if (encrypted == null || pinHash == null) {
        _shake();
        _showError('Profile data corrupted. Please recover from seed phrase.');
        setState(() => _isUnlocking = false);
        return;
      }

      // Verify PIN against stored hash
      debugPrint('UNLOCK: Verifying PIN...');
      final pinMatches = await identityService.verifyPin(pin, pinHash);

      debugPrint('UNLOCK: PIN matches: $pinMatches');

      if (pinMatches) {
        // Correct PIN - decrypt identity and initialize API
        try {
          debugPrint('UNLOCK: Decrypting identity...');
          final identity =
              await identityService.decryptIdentity(encrypted, pin);

          debugPrint('UNLOCK: Identity decrypted, ID: ${identity.id}');
          debugPrint('UNLOCK: Identity pubkey: ${identity.ed25519PubKey}');

          // Initialize API with identity
          debugPrint('UNLOCK: Setting identity on API...');
          await appState.api.setIdentity(identity);

          // Stop any existing SSE and start fresh
          appState.stopSSE();
          appState.startSSE();

          _pinCtrl.clear();
          _pinAttempts = 0;

          // Show welcome and unlock
          if (mounted) {
            ScaffoldMessenger.of(context).showSnackBar(
              SnackBar(
                content: Text('Welcome back, $_profileName!'),
                backgroundColor: RoyalTheme.green,
                duration: const Duration(seconds: 2),
              ),
            );
            widget.onUnlocked();
          }
        } catch (e) {
          debugPrint('UNLOCK: Decryption ERROR: $e');
          _shake();
          _showError('Decryption failed: $e');
        }
      } else {
        // Wrong PIN
        _pinAttempts++;
        _pinCtrl.clear();
        _shake();
        _startLockout();

        final nextLockout = _calculateLockoutDuration(_pinAttempts);
        String message;
        if (_pinAttempts == 1) {
          message = 'Wrong PIN. Next attempt in 30 seconds.';
        } else {
          message =
              'Wrong PIN. Next attempt in ${_formatDuration(nextLockout)}.';
        }

        if (mounted) {
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(content: Text(message), backgroundColor: RoyalTheme.red),
          );
        }
      }
    } catch (e) {
      debugPrint('UNLOCK: ERROR: $e');
      _shake();
      _showError('Error: $e');
    } finally {
      if (mounted) setState(() => _isUnlocking = false);
    }
  }

  String _formatDuration(int seconds) {
    if (seconds < 60) return '${seconds}s';
    if (seconds < 3600) return '${seconds ~/ 60}m ${seconds % 60}s';
    return '${seconds ~/ 3600}h ${(seconds % 3600) ~/ 60}m';
  }

  Future<void> _triggerPanic() async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        backgroundColor: RoyalTheme.darkCard,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(16),
          side: const BorderSide(color: RoyalTheme.red, width: 2),
        ),
        title: Row(
          children: [
            Container(
              padding: const EdgeInsets.all(8),
              decoration: BoxDecoration(
                color: RoyalTheme.red.withAlpha(30),
                shape: BoxShape.circle,
              ),
              child: const Icon(Icons.warning_amber_rounded,
                  color: RoyalTheme.red, size: 24),
            ),
            const SizedBox(width: 12),
            const Text('PANIC MODE',
                style: TextStyle(
                    color: RoyalTheme.red, fontWeight: FontWeight.bold)),
          ],
        ),
        content: const Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              'This will PERMANENTLY DISABLE PIN access for this profile.',
              style: TextStyle(color: Colors.white, fontWeight: FontWeight.w600),
            ),
            SizedBox(height: 12),
            Text(
              'This will also disable Emergency Stop and wipe all encrypted containers.',
              style: TextStyle(color: Colors.grey, fontSize: 13),
            ),
            SizedBox(height: 12),
            Text(
              'Are you absolutely sure?',
              style: TextStyle(color: RoyalTheme.red, fontWeight: FontWeight.bold),
            ),
          ],
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: Text('Cancel', style: TextStyle(color: Colors.grey[400])),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            style: FilledButton.styleFrom(
              backgroundColor: RoyalTheme.red,
              foregroundColor: Colors.white,
            ),
            child: const Text('ACTIVATE PANIC'),
          ),
        ],
      ),
    );

    if (confirmed == true) {
      try {
        final prefs = await SharedPreferences.getInstance();
        await prefs.setBool('${widget.profileId}_panic_mode', true);
        setState(() => _panicMode = true);

        if (mounted) {
          ScaffoldMessenger.of(context).showSnackBar(
            const SnackBar(
              content: Text(
                  'Panic mode activated. PIN access disabled. Recover from seed phrase to restore.'),
              backgroundColor: RoyalTheme.red,
              duration: Duration(seconds: 5),
            ),
          );
        }
      } catch (e) {
        _showError('Failed to activate panic mode: $e');
      }
    }
  }

  void _shake() {
    _shakeCtrl.forward(from: 0);
  }

  void _showError(String message) {
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(content: Text(message), backgroundColor: RoyalTheme.red),
    );
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Scaffold(
      backgroundColor: RoyalTheme.deepNavy,
      body: AnimatedBuilder(
        animation: _shakeAnim,
        builder: (context, child) {
          return Transform.translate(
            offset: Offset(_shakeAnim.value, 0),
            child: child,
          );
        },
        child: Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 420),
            child: Padding(
              padding: const EdgeInsets.all(24),
              child: SingleChildScrollView(
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    // Crown icon
                    Container(
                      width: 80,
                      height: 80,
                      decoration: BoxDecoration(
                        shape: BoxShape.circle,
                        border: Border.all(color: RoyalTheme.gold, width: 2),
                        boxShadow: [
                          BoxShadow(
                              color: RoyalTheme.gold.withAlpha(40),
                              blurRadius: 16)
                        ],
                      ),
                      child: const Center(
                        child: Text('♚',
                            style: TextStyle(
                                fontSize: 40, color: RoyalTheme.gold)),
                      ),
                    ),
                    const SizedBox(height: 24),

                    // Title
                    Text(
                      'Royal Control',
                      style: theme.textTheme.headlineMedium?.copyWith(
                        fontWeight: FontWeight.bold,
                        color: Colors.white,
                      ),
                    ),
                    const SizedBox(height: 6),
                    Text(
                      _profileName ?? 'Sovereign Admin Console',
                      style: theme.textTheme.bodyMedium
                          ?.copyWith(color: Colors.grey[400]),
                    ),
                    const SizedBox(height: 32),

                    // Panic mode indicator
                    if (_panicMode) ...[
                      Container(
                        width: double.infinity,
                        padding: const EdgeInsets.all(16),
                        decoration: BoxDecoration(
                          color: RoyalTheme.red.withAlpha(30),
                          borderRadius: BorderRadius.circular(12),
                          border: Border.all(
                              color: RoyalTheme.red.withAlpha(80), width: 2),
                        ),
                        child: Row(
                          children: [
                            const Icon(Icons.block_rounded,
                                color: RoyalTheme.red, size: 24),
                            const SizedBox(width: 12),
                            Expanded(
                              child: Column(
                                crossAxisAlignment: CrossAxisAlignment.start,
                                children: [
                                  Text(
                                    'PANIC MODE ACTIVE',
                                    style: TextStyle(
                                        color: RoyalTheme.red,
                                        fontWeight: FontWeight.bold,
                                        fontSize: 15),
                                  ),
                                  Text(
                                    'PIN access disabled. Recover from seed phrase to restore.',
                                    style: TextStyle(
                                        color: Colors.grey[300], fontSize: 13),
                                  ),
                                ],
                              ),
                            ),
                          ],
                        ),
                      ),
                      const SizedBox(height: 24),
                    ],

                    // Lockout indicator
                    if (_lockedOut) ...[
                      Container(
                        width: double.infinity,
                        padding: const EdgeInsets.symmetric(
                            horizontal: 16, vertical: 12),
                        decoration: BoxDecoration(
                          color: RoyalTheme.red.withAlpha(30),
                          borderRadius: BorderRadius.circular(12),
                          border:
                              Border.all(color: RoyalTheme.red.withAlpha(80)),
                        ),
                        child: Row(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            const Icon(Icons.timer_rounded,
                                color: RoyalTheme.red, size: 20),
                            const SizedBox(width: 10),
                            Text(
                              'Locked for ${_formatDuration(_remainingLockoutSeconds)}',
                              style: const TextStyle(
                                  color: RoyalTheme.red,
                                  fontWeight: FontWeight.w600,
                                  fontSize: 16),
                            ),
                          ],
                        ),
                      ),
                      const SizedBox(height: 24),
                    ],

                    // PIN field (only show if not in panic mode)
                    if (!_panicMode) ...[
                      Container(
                        decoration: BoxDecoration(
                          color: RoyalTheme.darkCard,
                          borderRadius: BorderRadius.circular(12),
                          border:
                              Border.all(color: RoyalTheme.gold.withAlpha(60)),
                        ),
                        padding: const EdgeInsets.symmetric(
                            horizontal: 16, vertical: 8),
                        child: Row(
                          children: [
                            const Icon(Icons.lock_rounded,
                                color: RoyalTheme.gold, size: 20),
                            const SizedBox(width: 12),
                            Expanded(
                              child: TextField(
                                controller: _pinCtrl,
                                obscureText: _obscurePin,
                                keyboardType: TextInputType.number,
                                maxLength: 8,
                                textAlign: TextAlign.center,
                                style: TextStyle(
                                  fontSize: 24,
                                  letterSpacing: _obscurePin ? 10 : 6,
                                  fontWeight: FontWeight.w500,
                                  color: Colors.white,
                                  fontFamily: 'monospace',
                                ),
                                decoration: const InputDecoration(
                                  border: InputBorder.none,
                                  counterText: '',
                                  hintText: '• • • • • • • •',
                                  hintStyle: TextStyle(
                                      color: Colors.grey,
                                      letterSpacing: 10,
                                      fontFamily: 'monospace'),
                                  isDense: true,
                                ),
                                inputFormatters: [
                                  FilteringTextInputFormatter.digitsOnly
                                ],
                                onSubmitted: (_) => _onUnlock(),
                                enabled: !_lockedOut && !_isUnlocking,
                              ),
                            ),
                            IconButton(
                              icon: Icon(
                                _obscurePin
                                    ? Icons.visibility_off_rounded
                                    : Icons.visibility_rounded,
                                color: Colors.grey[500],
                                size: 22,
                              ),
                              onPressed: () =>
                                  setState(() => _obscurePin = !_obscurePin),
                              padding: EdgeInsets.zero,
                              constraints: const BoxConstraints(),
                            ),
                          ],
                        ),
                      ),
                      const SizedBox(height: 24),

                      // Unlock button
                      SizedBox(
                        width: double.infinity,
                        height: 56,
                        child: FilledButton(
                          style: FilledButton.styleFrom(
                            backgroundColor: RoyalTheme.gold,
                            foregroundColor: Colors.black,
                            shape: RoundedRectangleBorder(
                                borderRadius: BorderRadius.circular(12)),
                            elevation: 4,
                            shadowColor: RoyalTheme.gold.withAlpha(60),
                          ),
                          onPressed:
                              (_lockedOut || _isUnlocking) ? null : _onUnlock,
                          child: _isUnlocking
                              ? const SizedBox(
                                  width: 24,
                                  height: 24,
                                  child: CircularProgressIndicator(
                                      strokeWidth: 2, color: Colors.black))
                              : const Text(
                                  'UNLOCK',
                                  style: TextStyle(
                                      fontSize: 16,
                                      fontWeight: FontWeight.bold,
                                      letterSpacing: 1.5),
                                ),
                        ),
                      ),
                      const SizedBox(height: 16),
                    ],

                    // Action buttons row
                    Row(
                      children: [
                        if (widget.onBackToProfiles != null)
                          Expanded(
                            child: OutlinedButton.icon(
                              onPressed: widget.onBackToProfiles,
                              icon: const Icon(Icons.person_rounded, size: 18),
                              label: const Text('Switch Profile'),
                              style: OutlinedButton.styleFrom(
                                foregroundColor: RoyalTheme.gold,
                                side: const BorderSide(color: RoyalTheme.gold),
                                padding:
                                    const EdgeInsets.symmetric(vertical: 14),
                                shape: RoundedRectangleBorder(
                                    borderRadius: BorderRadius.circular(12)),
                              ),
                            ),
                          ),
                        if (widget.onBackToProfiles != null)
                          const SizedBox(width: 12),
                        Expanded(
                          child: OutlinedButton.icon(
                            onPressed: widget.onRecoverFromSeed,
                            icon: const Icon(Icons.restore_rounded, size: 18),
                            label: const Text('Recover from Seed'),
                            style: OutlinedButton.styleFrom(
                              foregroundColor: RoyalTheme.gold,
                              side: const BorderSide(color: RoyalTheme.gold),
                              padding: const EdgeInsets.symmetric(vertical: 14),
                              shape: RoundedRectangleBorder(
                                  borderRadius: BorderRadius.circular(12)),
                            ),
                          ),
                        ),
                      ],
                    ),

                    const SizedBox(height: 16),

                    // Panic button
                    if (!_panicMode)
                      SizedBox(
                        width: double.infinity,
                        child: OutlinedButton.icon(
                          onPressed: _triggerPanic,
                          icon: const Icon(Icons.emergency_rounded, size: 18),
                          label:
                              const Text('PANIC BUTTON — Disable PIN Access'),
                          style: OutlinedButton.styleFrom(
                            foregroundColor: RoyalTheme.red,
                            side: const BorderSide(
                                color: RoyalTheme.red, width: 1.5),
                            padding: const EdgeInsets.symmetric(vertical: 14),
                            shape: RoundedRectangleBorder(
                                borderRadius: BorderRadius.circular(12)),
                          ),
                        ),
                      ),

                    const SizedBox(height: 24),

                    // Hint
                    Text(
                      _panicMode
                          ? 'PIN access disabled. Use "Recover from Seed" to restore.'
                          : '8-digit PIN • Wrong attempts: 30s → 60s → 120s → 240s... • Panic button disables PIN until seed recovery',
                      style: theme.textTheme.bodySmall
                          ?.copyWith(color: Colors.grey[600]),
                      textAlign: TextAlign.center,
                    ),

                    const SizedBox(height: 24),

                    // Version info
                    Container(
                      padding: const EdgeInsets.symmetric(
                          horizontal: 12, vertical: 6),
                      decoration: BoxDecoration(
                        color: Colors.black.withAlpha(80),
                        borderRadius: BorderRadius.circular(6),
                        border: Border.all(color: Colors.grey[800]!),
                      ),
                      child: Text(
                        'Royal Control v2.0.0 • Saint Mary Liberty Island',
                        style: TextStyle(
                            fontSize: 13,
                            color: Colors.grey[500],
                            fontFamily: 'monospace'),
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}
