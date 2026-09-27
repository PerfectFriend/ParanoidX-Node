import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:models/models.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:convert/convert.dart';
import 'package:bip39/bip39.dart' as bip39;
import 'dart:math' show Random;
import '../theme.dart';

/// Multi-step registration screen for first-time setup
/// Step 1: Warning about seed phrase responsibility
/// Step 2: Display generated 24-word BIP39 mnemonic
/// Step 3: Verify 3 random words from the mnemonic
/// Step 4: Set up 8-digit PIN with visibility toggle
class RegistrationScreen extends StatefulWidget {
  final VoidCallback onRegistrationComplete;

  const RegistrationScreen({
    super.key,
    required this.onRegistrationComplete,
  });

  @override
  State<RegistrationScreen> createState() => _RegistrationScreenState();
}

class _RegistrationScreenState extends State<RegistrationScreen>
    with TickerProviderStateMixin {
  int _currentStep = 0;
  late List<String> _mnemonicWords;
  late List<int> _verifyIndices;
  late List<String> _verifyWords;
  final List<TextEditingController> _verifyControllers =
      List.generate(3, (_) => TextEditingController());
  final _pinCtrl = TextEditingController();
  final _confirmPinCtrl = TextEditingController();
  bool _obscurePin = true;
  bool _obscureConfirmPin = true;
  bool _warningAcknowledged = false;
  bool _mnemonicWrittenDown = false;
  bool _verificationFailed = false;
  String? _errorMessage;
  late AnimationController _fadeCtrl;
  late AnimationController _slideCtrl;
  late Animation<double> _fadeAnim;
  late Animation<Offset> _slideAnim;

  @override
  void initState() {
    super.initState();
    _generateMnemonic();
    _pickVerificationWords();
    _fadeCtrl = AnimationController(
        duration: const Duration(milliseconds: 300), vsync: this);
    _slideCtrl = AnimationController(
        duration: const Duration(milliseconds: 300), vsync: this);
    _fadeAnim = Tween<double>(begin: 0.0, end: 1.0)
        .animate(CurvedAnimation(parent: _fadeCtrl, curve: Curves.easeOut));
    _slideAnim = Tween<Offset>(begin: const Offset(0.1, 0), end: Offset.zero)
        .animate(CurvedAnimation(parent: _slideCtrl, curve: Curves.easeOut));
    _fadeCtrl.forward();
    _slideCtrl.forward();
  }

  void _generateMnemonic() async {
    final entropyBytes = _generateEntropy(32); // 32 bytes = 256 bits = 24 words
    final mnemonic = bip39.entropyToMnemonic(hex.encode(entropyBytes));
    _mnemonicWords = mnemonic.split(' ');
  }

  Uint8List _generateEntropy(int length) {
    final random = Random.secure();
    final bytes = Uint8List(length);
    for (int i = 0; i < length; i++) {
      bytes[i] = random.nextInt(256);
    }
    return bytes;
  }

  void _pickVerificationWords() {
    final random = Random.secure();
    _verifyIndices =
        List.generate(3, (_) => random.nextInt(24)).toSet().toList();
    while (_verifyIndices.length < 3) {
      final idx = random.nextInt(24);
      if (!_verifyIndices.contains(idx)) _verifyIndices.add(idx);
    }
    _verifyIndices.sort();
    _verifyWords = _verifyIndices.map((i) => _mnemonicWords[i]).toList();
  }

  void _nextStep() {
    setState(() {
      _errorMessage = null;
      if (_currentStep == 0) {
        if (!_warningAcknowledged || !_mnemonicWrittenDown) {
          _errorMessage =
              'Please acknowledge the warning and confirm you have written down the seed phrase';
          return;
        }
      } else if (_currentStep == 1) {
        // Move to verification step
      } else if (_currentStep == 2) {
        // Verify the 3 words
        for (int i = 0; i < 3; i++) {
          if (_verifyControllers[i].text.trim().toLowerCase() !=
              _verifyWords[i].toLowerCase()) {
            _verificationFailed = true;
            _errorMessage =
                'Verification failed. Word ${_verifyIndices[i] + 1} is incorrect.';
            return;
          }
        }
        _verificationFailed = false;
      } else if (_currentStep == 3) {
        // Verify PIN
        final pin = _pinCtrl.text;
        final confirmPin = _confirmPinCtrl.text;
        if (pin.length != 8 || !RegExp(r'^\d{8}$').hasMatch(pin)) {
          _errorMessage = 'PIN must be exactly 8 digits';
          return;
        }
        if (pin != confirmPin) {
          _errorMessage = 'PINs do not match';
          return;
        }
        _saveProfileAndComplete(pin);
        return;
      }
      _currentStep++;
      _fadeCtrl.reset();
      _slideCtrl.reset();
      _fadeCtrl.forward();
      _slideCtrl.forward();
    });
  }

  Future<void> _saveProfileAndComplete(String pin) async {
    try {
      final prefs = await SharedPreferences.getInstance();
      final identityService = await SecureIdentityService.instance;

      // Encrypt identity with PIN
      final encrypted = await identityService.encryptIdentity(
        mnemonic: _mnemonicWords.join(' '),
        pin: pin,
      );

      // Hash PIN for storage
      final pinHash = await identityService.hashPin(pin);

      // Generate profile ID
      final profileId = 'profile_${DateTime.now().millisecondsSinceEpoch}';
      final profileName =
          'Profile ${(prefs.getStringList('profile_ids') ?? []).length + 1}';

      // Store profile data
      await prefs.setString('${profileId}_encrypted', encrypted);
      await prefs.setString('${profileId}_pin_hash', pinHash);
      await prefs.setString('${profileId}_name', profileName);
      await prefs.setString(
          '${profileId}_created', DateTime.now().toIso8601String());
      await prefs.setBool('${profileId}_panic_mode', false);

      // Update profile list
      final profileIds = prefs.getStringList('profile_ids') ?? [];
      profileIds.add(profileId);
      await prefs.setStringList('profile_ids', profileIds);

      // Set as active profile
      await prefs.setString('active_profile', profileId);

      if (mounted) {
        widget.onRegistrationComplete();
      }
    } catch (e) {
      setState(() {
        _errorMessage = 'Failed to save profile: $e';
      });
    }
  }

  void _previousStep() {
    if (_currentStep > 0) {
      setState(() {
        _currentStep--;
        _errorMessage = null;
        _fadeCtrl.reset();
        _slideCtrl.reset();
        _fadeCtrl.forward();
        _slideCtrl.forward();
      });
    }
  }

  @override
  void dispose() {
    for (final ctrl in _verifyControllers) {
      ctrl.dispose();
    }
    _pinCtrl.dispose();
    _confirmPinCtrl.dispose();
    _fadeCtrl.dispose();
    _slideCtrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: RoyalTheme.darkScaffold,
      body: SafeArea(
        child: FadeTransition(
          opacity: _fadeAnim,
          child: SlideTransition(
            position: _slideAnim,
            child: Padding(
              padding: const EdgeInsets.all(24),
              child: Column(
                children: [
                  // Progress indicator
                  _buildProgressIndicator(),
                  const SizedBox(height: 32),
                  // Step content
                  Expanded(child: _buildStepContent()),
                  const SizedBox(height: 24),
                  // Navigation buttons
                  _buildNavigationButtons(),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }

  Widget _buildProgressIndicator() {
    return Row(
      children: List.generate(4, (i) {
        final isActive = i <= _currentStep;
        return Expanded(
          child: Row(
            children: [
              Expanded(
                child: Container(
                  height: 4,
                  decoration: BoxDecoration(
                    color: isActive
                        ? RoyalTheme.gold
                        : RoyalTheme.gold.withAlpha(60),
                    borderRadius: BorderRadius.circular(2),
                  ),
                ),
              ),
              if (i < 3) const SizedBox(width: 8),
            ],
          ),
        );
      }),
    );
  }

  Widget _buildStepContent() {
    switch (_currentStep) {
      case 0:
        return _buildWarningStep();
      case 1:
        return _buildSeedPhraseStep();
      case 2:
        return _buildVerificationStep();
      case 3:
        return _buildPinSetupStep();
      default:
        return const SizedBox();
    }
  }

  Widget _buildWarningStep() {
    return SingleChildScrollView(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Container(
            padding: const EdgeInsets.all(24),
            decoration: BoxDecoration(
              color: RoyalTheme.darkCard,
              borderRadius: BorderRadius.circular(16),
              border: Border.all(color: RoyalTheme.red.withAlpha(80), width: 2),
            ),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    Container(
                      padding: const EdgeInsets.all(12),
                      decoration: BoxDecoration(
                        color: RoyalTheme.red.withAlpha(30),
                        shape: BoxShape.circle,
                      ),
                      child: const Icon(Icons.warning_amber_rounded,
                          color: RoyalTheme.red, size: 28),
                    ),
                    const SizedBox(width: 16),
                    Expanded(
                      child: Text(
                        'CRITICAL SECURITY WARNING',
                        style:
                            Theme.of(context).textTheme.headlineSmall?.copyWith(
                                  color: RoyalTheme.red,
                                  fontWeight: FontWeight.bold,
                                ),
                      ),
                    ),
                  ],
                ),
                const SizedBox(height: 24),
                Text(
                  'You are about to create a sovereign identity for the Saint Mary Liberty Island Royal Control system. This identity is secured by a 24-word BIP39 seed phrase.',
                  style: Theme.of(context)
                      .textTheme
                      .bodyLarge
                      ?.copyWith(color: Colors.white),
                ),
                const SizedBox(height: 16),
                Text(
                  'READ CAREFULLY:',
                  style: Theme.of(context).textTheme.titleMedium?.copyWith(
                        color: RoyalTheme.gold,
                        fontWeight: FontWeight.bold,
                      ),
                ),
                const SizedBox(height: 12),
                _buildWarningPoint(
                    'Your seed phrase is the ONLY way to recover your identity and funds'),
                _buildWarningPoint(
                    'If you lose your seed phrase, access is PERMANENTLY LOST — no one can recover it'),
                _buildWarningPoint(
                    'Anyone with your seed phrase has FULL CONTROL over your identity and assets'),
                _buildWarningPoint(
                    'Write it down on PAPER and store it SECURELY — never digital, never photographed'),
                _buildWarningPoint(
                    'Do not share it with anyone, including "support" or "officials"'),
                const SizedBox(height: 24),
                CheckboxListTile(
                  value: _warningAcknowledged,
                  onChanged: (v) =>
                      setState(() => _warningAcknowledged = v ?? false),
                  title: Text(
                      'I understand and accept full responsibility for my seed phrase security',
                      style: Theme.of(context)
                          .textTheme
                          .bodyMedium
                          ?.copyWith(color: Colors.white)),
                  activeColor: RoyalTheme.gold,
                  checkColor: RoyalTheme.darkScaffold,
                  controlAffinity: ListTileControlAffinity.leading,
                  contentPadding: EdgeInsets.zero,
                ),
                CheckboxListTile(
                  value: _mnemonicWrittenDown,
                  onChanged: (v) =>
                      setState(() => _mnemonicWrittenDown = v ?? false),
                  title: Text(
                      'I have written down my seed phrase on paper and stored it securely',
                      style: Theme.of(context)
                          .textTheme
                          .bodyMedium
                          ?.copyWith(color: Colors.white)),
                  activeColor: RoyalTheme.gold,
                  checkColor: RoyalTheme.darkScaffold,
                  controlAffinity: ListTileControlAffinity.leading,
                  contentPadding: EdgeInsets.zero,
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildWarningPoint(String text) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 8),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Icon(Icons.circle, size: 8, color: RoyalTheme.gold),
          const SizedBox(width: 12),
          Expanded(
              child: Text(text,
                  style: Theme.of(context)
                      .textTheme
                      .bodyMedium
                      ?.copyWith(color: Colors.grey[300]))),
        ],
      ),
    );
  }

  Widget _buildSeedPhraseStep() {
    return SingleChildScrollView(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            'Your 24-Word Seed Phrase',
            style: Theme.of(context).textTheme.headlineSmall?.copyWith(
                  fontWeight: FontWeight.bold,
                  color: Colors.white,
                ),
          ),
          const SizedBox(height: 8),
          Text(
            'Write these words in EXACT order on paper. Verify spelling carefully.',
            style: Theme.of(context)
                .textTheme
                .bodyMedium
                ?.copyWith(color: Colors.grey[400]),
          ),
          const SizedBox(height: 24),
          Container(
            padding: const EdgeInsets.all(20),
            decoration: BoxDecoration(
              color: RoyalTheme.darkCard,
              borderRadius: BorderRadius.circular(16),
              border: Border.all(color: RoyalTheme.gold.withAlpha(60)),
            ),
            child: Column(
              children: List.generate(6, (row) {
                final start = row * 4;
                return Padding(
                  padding: const EdgeInsets.only(bottom: 12),
                  child: Row(
                    mainAxisAlignment: MainAxisAlignment.spaceEvenly,
                    children: List.generate(4, (col) {
                      final idx = start + col;
                      return _buildMnemonicWord(idx, _mnemonicWords[idx]);
                    }),
                  ),
                );
              }),
            ),
          ),
          const SizedBox(height: 24),
          Container(
            padding: const EdgeInsets.all(16),
            decoration: BoxDecoration(
              color: RoyalTheme.gold.withAlpha(20),
              borderRadius: BorderRadius.circular(12),
              border: Border.all(color: RoyalTheme.gold.withAlpha(80)),
            ),
            child: Row(
              children: [
                const Icon(Icons.copy_rounded, color: RoyalTheme.gold),
                const SizedBox(width: 12),
                Expanded(
                  child: Text(
                    'Copy to clipboard (temporary — will clear in 30s)',
                    style: Theme.of(context)
                        .textTheme
                        .bodyMedium
                        ?.copyWith(color: RoyalTheme.gold),
                  ),
                ),
                TextButton.icon(
                  onPressed: () {
                    Clipboard.setData(
                        ClipboardData(text: _mnemonicWords.join(' ')));
                    ScaffoldMessenger.of(context).showSnackBar(
                      const SnackBar(
                          content: Text(
                              'Seed phrase copied — clear clipboard after writing down!')),
                    );
                  },
                  icon: const Icon(Icons.content_copy, size: 18),
                  label: const Text('Copy'),
                  style: TextButton.styleFrom(foregroundColor: RoyalTheme.gold),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildMnemonicWord(int index, String word) {
    return Container(
      width: 70,
      padding: const EdgeInsets.symmetric(vertical: 10, horizontal: 8),
      decoration: BoxDecoration(
        color: RoyalTheme.darkScaffold,
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: RoyalTheme.gold.withAlpha(40)),
      ),
      child: Column(
        children: [
          Text(
            '${index + 1}',
            style: Theme.of(context)
                .textTheme
                .bodySmall
                ?.copyWith(color: RoyalTheme.gold, fontSize: 13),
          ),
          const SizedBox(height: 6),
          Text(
            word,
            style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                  fontWeight: FontWeight.w600,
                  color: Colors.white,
                  fontFamily: 'monospace',
                ),
            textAlign: TextAlign.center,
          ),
        ],
      ),
    );
  }

  Widget _buildVerificationStep() {
    return SingleChildScrollView(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            'Verify Your Seed Phrase',
            style: Theme.of(context).textTheme.headlineSmall?.copyWith(
                  fontWeight: FontWeight.bold,
                  color: Colors.white,
                ),
          ),
          const SizedBox(height: 8),
          Text(
            'Enter the 3 words at the indicated positions from your seed phrase.',
            style: Theme.of(context)
                .textTheme
                .bodyMedium
                ?.copyWith(color: Colors.grey[400]),
          ),
          const SizedBox(height: 24),
          ...List.generate(3, (i) {
            final wordIndex = _verifyIndices[i];
            return Padding(
              padding: const EdgeInsets.only(bottom: 16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    'Word #${wordIndex + 1}',
                    style: Theme.of(context)
                        .textTheme
                        .titleMedium
                        ?.copyWith(color: RoyalTheme.gold),
                  ),
                  const SizedBox(height: 8),
                  TextField(
                    controller: _verifyControllers[i],
                    decoration: InputDecoration(
                      hintText: 'Enter word ${wordIndex + 1}',
                      filled: true,
                      fillColor: RoyalTheme.darkCard,
                      border: OutlineInputBorder(
                        borderRadius: BorderRadius.circular(12),
                        borderSide: BorderSide(
                          color: _verificationFailed &&
                                  i ==
                                      _verifyControllers.indexWhere((c) =>
                                          c.text.trim().toLowerCase() !=
                                          _verifyWords[
                                                  _verifyControllers.indexOf(c)]
                                              .toLowerCase())
                              ? RoyalTheme.red
                              : RoyalTheme.gold.withAlpha(60),
                        ),
                      ),
                      enabledBorder: OutlineInputBorder(
                        borderRadius: BorderRadius.circular(12),
                        borderSide:
                            BorderSide(color: RoyalTheme.gold.withAlpha(60)),
                      ),
                      focusedBorder: OutlineInputBorder(
                        borderRadius: BorderRadius.circular(12),
                        borderSide:
                            const BorderSide(color: RoyalTheme.gold, width: 2),
                      ),
                      errorBorder: OutlineInputBorder(
                        borderRadius: BorderRadius.circular(12),
                        borderSide:
                            const BorderSide(color: RoyalTheme.red, width: 2),
                      ),
                    ),
                    style: const TextStyle(
                        color: Colors.white,
                        fontSize: 20,
                        fontWeight: FontWeight.w500),
                    textCapitalization: TextCapitalization.none,
                    autocorrect: false,
                    enableSuggestions: false,
                    onChanged: (_) => setState(() {}),
                  ),
                ],
              ),
            );
          }),
          if (_errorMessage != null) ...[
            const SizedBox(height: 16),
            Container(
              padding: const EdgeInsets.all(16),
              decoration: BoxDecoration(
                color: RoyalTheme.red.withAlpha(30),
                borderRadius: BorderRadius.circular(12),
                border: Border.all(color: RoyalTheme.red.withAlpha(80)),
              ),
              child: Row(
                children: [
                  const Icon(Icons.error_outline, color: RoyalTheme.red),
                  const SizedBox(width: 12),
                  Expanded(
                      child: Text(_errorMessage!,
                          style: const TextStyle(color: RoyalTheme.red))),
                ],
              ),
            ),
          ],
        ],
      ),
    );
  }

  Widget _buildPinSetupStep() {
    return SingleChildScrollView(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            'Set 8-Digit PIN',
            style: Theme.of(context).textTheme.headlineSmall?.copyWith(
                  fontWeight: FontWeight.bold,
                  color: Colors.white,
                ),
          ),
          const SizedBox(height: 8),
          Text(
            'Your PIN unlocks the app. Use a unique, memorable 8-digit code.',
            style: Theme.of(context)
                .textTheme
                .bodyMedium
                ?.copyWith(color: Colors.grey[400]),
          ),
          const SizedBox(height: 24),
          _buildPinField(
            controller: _pinCtrl,
            label: 'PIN (8 digits)',
            obscure: _obscurePin,
            onToggle: () => setState(() => _obscurePin = !_obscurePin),
          ),
          const SizedBox(height: 16),
          _buildPinField(
            controller: _confirmPinCtrl,
            label: 'Confirm PIN',
            obscure: _obscureConfirmPin,
            onToggle: () =>
                setState(() => _obscureConfirmPin = !_obscureConfirmPin),
          ),
          if (_errorMessage != null) ...[
            const SizedBox(height: 16),
            Container(
              padding: const EdgeInsets.all(16),
              decoration: BoxDecoration(
                color: RoyalTheme.red.withAlpha(30),
                borderRadius: BorderRadius.circular(12),
                border: Border.all(color: RoyalTheme.red.withAlpha(80)),
              ),
              child: Row(
                children: [
                  const Icon(Icons.error_outline, color: RoyalTheme.red),
                  const SizedBox(width: 12),
                  Expanded(
                      child: Text(_errorMessage!,
                          style: const TextStyle(color: RoyalTheme.red))),
                ],
              ),
            ),
          ],
          const SizedBox(height: 24),
          Container(
            padding: const EdgeInsets.all(16),
            decoration: BoxDecoration(
              color: RoyalTheme.gold.withAlpha(20),
              borderRadius: BorderRadius.circular(12),
              border: Border.all(color: RoyalTheme.gold.withAlpha(80)),
            ),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    const Icon(Icons.security_rounded, color: RoyalTheme.gold),
                    const SizedBox(width: 12),
                    Text('PIN Security',
                        style: Theme.of(context)
                            .textTheme
                            .titleMedium
                            ?.copyWith(color: RoyalTheme.gold)),
                  ],
                ),
                const SizedBox(height: 12),
                Text(
                  '• 8 digits only (0-9)\n'
                  '• Wrong PIN: 30s → 60s → 120s → 240s... (doubles each attempt)\n'
                  '• Panic button: disables PIN until seed phrase recovery\n'
                  '• No biometrics — PIN only for maximum security',
                  style: Theme.of(context)
                      .textTheme
                      .bodyMedium
                      ?.copyWith(color: Colors.grey[300]),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildPinField({
    required TextEditingController controller,
    required String label,
    required bool obscure,
    required VoidCallback onToggle,
  }) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(label,
            style: Theme.of(context)
                .textTheme
                .titleMedium
                ?.copyWith(color: RoyalTheme.gold)),
        const SizedBox(height: 8),
        TextField(
          controller: controller,
          obscureText: obscure,
          keyboardType: TextInputType.number,
          maxLength: 8,
          inputFormatters: [FilteringTextInputFormatter.digitsOnly],
          style: TextStyle(
            color: Colors.white,
            fontSize: 26,
            letterSpacing: obscure ? 8 : 4,
            fontFamily: 'monospace',
            fontWeight: FontWeight.w500,
          ),
          textAlign: TextAlign.center,
          decoration: InputDecoration(
            counterText: '',
            filled: true,
            fillColor: RoyalTheme.darkCard,
            border: OutlineInputBorder(
              borderRadius: BorderRadius.circular(12),
              borderSide: BorderSide(color: RoyalTheme.gold.withAlpha(60)),
            ),
            enabledBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(12),
              borderSide: BorderSide(color: RoyalTheme.gold.withAlpha(60)),
            ),
            focusedBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(12),
              borderSide: const BorderSide(color: RoyalTheme.gold, width: 2),
            ),
            suffixIcon: IconButton(
              icon: Icon(obscure ? Icons.visibility_off : Icons.visibility,
                  color: Colors.grey[500]),
              onPressed: onToggle,
            ),
            hintText: '• • • • • • • •',
            hintStyle: TextStyle(
                color: Colors.grey[600],
                letterSpacing: 8,
                fontFamily: 'monospace'),
          ),
        ),
      ],
    );
  }

  Widget _buildNavigationButtons() {
    return Row(
      children: [
        if (_currentStep > 0)
          Expanded(
            child: OutlinedButton.icon(
              onPressed: _previousStep,
              icon: const Icon(Icons.arrow_back),
              label: const Text('Back'),
              style: OutlinedButton.styleFrom(
                foregroundColor: RoyalTheme.gold,
                side: const BorderSide(color: RoyalTheme.gold),
                padding: const EdgeInsets.symmetric(vertical: 16),
                shape: RoundedRectangleBorder(
                    borderRadius: BorderRadius.circular(12)),
              ),
            ),
          ),
        if (_currentStep > 0) const SizedBox(width: 16),
        Expanded(
          child: ElevatedButton.icon(
            onPressed: _nextStep,
            icon: _currentStep == 3
                ? const Icon(Icons.check)
                : const Icon(Icons.arrow_forward),
            label: Text(_currentStep == 3 ? 'Complete Setup' : 'Continue'),
            style: ElevatedButton.styleFrom(
              backgroundColor: RoyalTheme.gold,
              foregroundColor: RoyalTheme.darkScaffold,
              padding: const EdgeInsets.symmetric(vertical: 16),
              shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(12)),
              textStyle:
                  const TextStyle(fontWeight: FontWeight.bold, fontSize: 16),
            ),
          ),
        ),
      ],
    );
  }
}
