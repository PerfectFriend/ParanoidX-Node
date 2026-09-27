import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:models/models.dart';
import 'package:shared_preferences/shared_preferences.dart';
import '../theme.dart';

class ProfileInfo {
  final String id;
  final String name;
  final DateTime created;
  final bool panicMode;

  ProfileInfo(
      {required this.id,
      required this.name,
      required this.created,
      required this.panicMode});
}

class ProfileSelectionScreen extends StatefulWidget {
  final List<ProfileInfo> profiles;
  final Function(String) onProfileSelected;
  final VoidCallback onRecoverFromSeed;
  final VoidCallback onCreateNewProfile;

  const ProfileSelectionScreen({
    super.key,
    required this.profiles,
    required this.onProfileSelected,
    required this.onRecoverFromSeed,
    required this.onCreateNewProfile,
  });

  @override
  State<ProfileSelectionScreen> createState() => _ProfileSelectionScreenState();
}

class _ProfileSelectionScreenState extends State<ProfileSelectionScreen>
    with TickerProviderStateMixin {
  late AnimationController _fadeCtrl;
  late AnimationController _staggerCtrl;
  late Animation<double> _fadeAnim;

  @override
  void initState() {
    super.initState();
    _fadeCtrl = AnimationController(
        duration: const Duration(milliseconds: 400), vsync: this);
    _staggerCtrl = AnimationController(
        duration: const Duration(milliseconds: 600), vsync: this);
    _fadeAnim = Tween<double>(begin: 0.0, end: 1.0)
        .animate(CurvedAnimation(parent: _fadeCtrl, curve: Curves.easeOut));
    _fadeCtrl.forward();
    _staggerCtrl.forward();
  }

  @override
  void dispose() {
    _fadeCtrl.dispose();
    _staggerCtrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: RoyalTheme.darkScaffold,
      body: SafeArea(
        child: FadeTransition(
          opacity: _fadeAnim,
          child: Padding(
            padding: const EdgeInsets.all(24),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const SizedBox(height: 16),
                Row(
                  children: [
                    Container(
                      width: 56,
                      height: 56,
                      decoration: BoxDecoration(
                        shape: BoxShape.circle,
                        border: Border.all(color: RoyalTheme.gold, width: 2),
                        boxShadow: [
                          BoxShadow(
                              color: RoyalTheme.gold.withAlpha(40),
                              blurRadius: 12)
                        ],
                      ),
                      child: const Center(
                          child: Text('♚',
                              style: TextStyle(
                                  fontSize: 28, color: RoyalTheme.gold))),
                    ),
                    const SizedBox(width: 16),
                    Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text('Royal Control',
                            style: Theme.of(context)
                                .textTheme
                                .headlineSmall
                                ?.copyWith(
                                    fontWeight: FontWeight.bold,
                                    color: Colors.white)),
                        Text('Select Profile',
                            style: Theme.of(context)
                                .textTheme
                                .bodyMedium
                                ?.copyWith(color: Colors.grey[400])),
                      ],
                    ),
                  ],
                ),
                const SizedBox(height: 32),
                Text('Saved Profiles',
                    style: Theme.of(context)
                        .textTheme
                        .titleMedium
                        ?.copyWith(color: RoyalTheme.gold)),
                const SizedBox(height: 16),
                Expanded(
                  child: ListView.builder(
                    itemCount: widget.profiles.length,
                    itemBuilder: (context, index) {
                      final profile = widget.profiles[index];
                      return _buildProfileCard(profile, index);
                    },
                  ),
                ),
                const SizedBox(height: 16),
                _buildActionButtons(),
              ],
            ),
          ),
        ),
      ),
    );
  }

  Widget _buildProfileCard(ProfileInfo profile, int index) {
    final isPanic = profile.panicMode;
    return TweenAnimationBuilder<double>(
      tween: Tween(begin: 0.0, end: 1.0),
      duration: Duration(milliseconds: 300 + index * 100),
      builder: (context, value, child) {
        return Transform.translate(
          offset: Offset(0, 20 * (1 - value)),
          child: Opacity(opacity: value, child: child),
        );
      },
      child: Container(
        margin: const EdgeInsets.only(bottom: 12),
        decoration: BoxDecoration(
          color: isPanic ? RoyalTheme.red.withAlpha(30) : RoyalTheme.darkCard,
          borderRadius: BorderRadius.circular(16),
          border: Border.all(
            color: isPanic
                ? RoyalTheme.red.withAlpha(100)
                : RoyalTheme.gold.withAlpha(60),
            width: isPanic ? 2 : 1,
          ),
        ),
        child: ListTile(
          contentPadding:
              const EdgeInsets.symmetric(horizontal: 20, vertical: 8),
          leading: Container(
            width: 48,
            height: 48,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              gradient: LinearGradient(
                colors: isPanic
                    ? [
                        RoyalTheme.red.withAlpha(80),
                        RoyalTheme.red.withAlpha(40)
                      ]
                    : [
                        RoyalTheme.gold.withAlpha(80),
                        RoyalTheme.gold.withAlpha(40)
                      ],
              ),
              border: Border.all(
                  color: isPanic ? RoyalTheme.red : RoyalTheme.gold,
                  width: 1.5),
            ),
            child: Center(
              child: Icon(
                  isPanic ? Icons.warning_amber_rounded : Icons.person_rounded,
                  color: isPanic ? RoyalTheme.red : RoyalTheme.gold,
                  size: 24),
            ),
          ),
          title: Text(profile.name,
              style: Theme.of(context).textTheme.titleMedium?.copyWith(
                    fontWeight: FontWeight.bold,
                    color: isPanic ? RoyalTheme.red : Colors.white,
                  )),
          subtitle: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const SizedBox(height: 4),
              Text('Created: ${_formatDate(profile.created)}',
                  style: Theme.of(context)
                      .textTheme
                      .bodySmall
                      ?.copyWith(color: Colors.grey[500])),
              if (isPanic)
                Text(
                    '⚠️ PANIC MODE ACTIVE — PIN disabled, seed recovery required',
                    style: Theme.of(context).textTheme.bodySmall?.copyWith(
                        color: RoyalTheme.red, fontWeight: FontWeight.w500)),
            ],
          ),
          trailing: isPanic
              ? const Icon(Icons.lock_rounded, color: RoyalTheme.red, size: 28)
              : Icon(Icons.arrow_forward_ios_rounded,
                  color: RoyalTheme.gold.withAlpha(180), size: 20),
          onTap: isPanic ? null : () => widget.onProfileSelected(profile.id),
        ),
      ),
    );
  }

  String _formatDate(DateTime date) {
    return '${date.day.toString().padLeft(2, '0')}.${date.month.toString().padLeft(2, '0')}.${date.year}';
  }

  Widget _buildActionButtons() {
    return Column(
      children: [
        OutlinedButton.icon(
          onPressed: widget.onRecoverFromSeed,
          icon: const Icon(Icons.restore_rounded),
          label: const Text('Recover from Seed Phrase'),
          style: OutlinedButton.styleFrom(
            foregroundColor: RoyalTheme.gold,
            side: const BorderSide(color: RoyalTheme.gold),
            padding: const EdgeInsets.symmetric(vertical: 16),
            minimumSize: const Size(double.infinity, 0),
            shape:
                RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
          ),
        ),
        const SizedBox(height: 12),
        OutlinedButton.icon(
          onPressed: widget.onCreateNewProfile,
          icon: const Icon(Icons.add_rounded),
          label: const Text('Create New Profile'),
          style: OutlinedButton.styleFrom(
            foregroundColor: Colors.grey[400],
            side: BorderSide(color: Colors.grey[600]!),
            padding: const EdgeInsets.symmetric(vertical: 16),
            minimumSize: const Size(double.infinity, 0),
            shape:
                RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
          ),
        ),
      ],
    );
  }
}

/// Screen for recovering a profile from seed phrase
class SeedRecoveryScreen extends StatefulWidget {
  final VoidCallback onRecoveryComplete;
  final VoidCallback onBack;

  const SeedRecoveryScreen({
    super.key,
    required this.onRecoveryComplete,
    required this.onBack,
  });

  @override
  State<SeedRecoveryScreen> createState() => _SeedRecoveryScreenState();
}

class _SeedRecoveryScreenState extends State<SeedRecoveryScreen>
    with TickerProviderStateMixin {
  final _mnemonicCtrl = TextEditingController();
  final _pinCtrl = TextEditingController();
  final _confirmPinCtrl = TextEditingController();
  bool _obscurePin = true;
  bool _obscureConfirmPin = true;
  String? _errorMessage;
  bool _isRecovering = false;
  late AnimationController _fadeCtrl;
  late Animation<double> _fadeAnim;

  @override
  void initState() {
    super.initState();
    _fadeCtrl = AnimationController(
        duration: const Duration(milliseconds: 400), vsync: this);
    _fadeAnim = Tween<double>(begin: 0.0, end: 1.0)
        .animate(CurvedAnimation(parent: _fadeCtrl, curve: Curves.easeOut));
    _fadeCtrl.forward();
  }

  @override
  void dispose() {
    _mnemonicCtrl.dispose();
    _pinCtrl.dispose();
    _confirmPinCtrl.dispose();
    _fadeCtrl.dispose();
    super.dispose();
  }

  Future<void> _recoverProfile() async {
    final mnemonic = _mnemonicCtrl.text.trim();
    final pin = _pinCtrl.text;
    final confirmPin = _confirmPinCtrl.text;

    debugPrint('RECOVER: Starting seed recovery');
    debugPrint(
        'RECOVER: Mnemonic words: ${mnemonic.split(RegExp(r'\\s+')).length}');
    debugPrint('RECOVER: PIN length: ${pin.length}');

    if (mnemonic.isEmpty) {
      setState(() => _errorMessage = 'Please enter your seed phrase');
      return;
    }

    final words = mnemonic.split(RegExp(r'\\s+'));
    if (words.length != 12 && words.length != 24) {
      setState(() => _errorMessage = 'Seed phrase must be 12 or 24 words');
      return;
    }

    if (pin.length != 8 || !RegExp(r'^\\d{8}$').hasMatch(pin)) {
      setState(() => _errorMessage = 'PIN must be exactly 8 digits');
      return;
    }

    if (pin != confirmPin) {
      setState(() => _errorMessage = 'PINs do not match');
      return;
    }

    setState(() {
      _isRecovering = true;
      _errorMessage = null;
    });

    try {
      final prefs = await SharedPreferences.getInstance();
      final identityService = await SecureIdentityService.instance;

      // Verify the mnemonic can create a valid identity
      debugPrint('RECOVER: Creating identity from mnemonic...');
      final identity =
          await identityService.createIdentityFromMnemonic(mnemonic: mnemonic);

      debugPrint('RECOVER: Identity created, ID: ${identity.id}');
      debugPrint('RECOVER: Identity pubkey: ${identity.ed25519PubKey}');

      // Encrypt with new PIN
      debugPrint('RECOVER: Encrypting mnemonic with new PIN...');
      final encrypted =
          await identityService.encryptIdentity(mnemonic: mnemonic, pin: pin);
      final pinHash = await identityService.hashPin(pin);

      debugPrint('RECOVER: Encrypted data length: ${encrypted.length}');
      debugPrint('RECOVER: PIN hash length: ${pinHash.length}');

      // Generate new profile ID
      final profileId = 'profile_${DateTime.now().millisecondsSinceEpoch}';
      final profileName =
          'Recovered Profile ${(prefs.getStringList('profile_ids') ?? []).length + 1}';

      await prefs.setString('${profileId}_encrypted', encrypted);
      await prefs.setString('${profileId}_pin_hash', pinHash);
      await prefs.setString('${profileId}_name', profileName);
      await prefs.setString(
          '${profileId}_created', DateTime.now().toIso8601String());
      await prefs.setBool('${profileId}_panic_mode', false);

      final profileIds = prefs.getStringList('profile_ids') ?? [];
      profileIds.add(profileId);
      await prefs.setStringList('profile_ids', profileIds);
      await prefs.setString('active_profile', profileId);

      debugPrint('RECOVER: Profile saved, ID: $profileId');
      debugPrint('RECOVER: Active profile set');

      if (mounted) {
        widget.onRecoveryComplete();
      }
    } catch (e) {
      debugPrint('RECOVER: ERROR: $e');
      setState(() {
        _isRecovering = false;
        _errorMessage = 'Recovery failed: $e';
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: RoyalTheme.darkScaffold,
      body: SafeArea(
        child: FadeTransition(
          opacity: _fadeAnim,
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(24),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    IconButton(
                      icon: const Icon(Icons.arrow_back_rounded,
                          color: RoyalTheme.gold),
                      onPressed: widget.onBack,
                    ),
                    const SizedBox(width: 8),
                    Text('Recover from Seed',
                        style:
                            Theme.of(context).textTheme.headlineSmall?.copyWith(
                                  fontWeight: FontWeight.bold,
                                  color: Colors.white,
                                )),
                  ],
                ),
                const SizedBox(height: 8),
                Text(
                  'Enter your 12 or 24 word BIP39 seed phrase to recover your identity',
                  style: Theme.of(context)
                      .textTheme
                      .bodyMedium
                      ?.copyWith(color: Colors.grey[400]),
                ),
                const SizedBox(height: 32),

                // Seed phrase input
                Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text('Seed Phrase',
                        style: Theme.of(context)
                            .textTheme
                            .titleMedium
                            ?.copyWith(color: RoyalTheme.gold)),
                    const SizedBox(height: 8),
                    TextField(
                      controller: _mnemonicCtrl,
                      maxLines: 4,
                      minLines: 3,
                      style: const TextStyle(
                          color: Colors.white,
                          fontFamily: 'monospace',
                          fontSize: 15),
                      decoration: InputDecoration(
                        hintText: 'word1 word2 word3 ...',
                        hintStyle: TextStyle(
                            color: Colors.grey[600], fontFamily: 'monospace'),
                        filled: true,
                        fillColor: RoyalTheme.darkCard,
                        border: OutlineInputBorder(
                          borderRadius: BorderRadius.circular(12),
                          borderSide:
                              BorderSide(color: RoyalTheme.gold.withAlpha(60)),
                        ),
                        enabledBorder: OutlineInputBorder(
                          borderRadius: BorderRadius.circular(12),
                          borderSide:
                              BorderSide(color: RoyalTheme.gold.withAlpha(60)),
                        ),
                        focusedBorder: OutlineInputBorder(
                          borderRadius: BorderRadius.circular(12),
                          borderSide: const BorderSide(
                              color: RoyalTheme.gold, width: 2),
                        ),
                      ),
                    ),
                  ],
                ),
                const SizedBox(height: 24),

                // New PIN
                _buildPinField(
                  controller: _pinCtrl,
                  label: 'New PIN (8 digits)',
                  obscure: _obscurePin,
                  onToggle: () => setState(() => _obscurePin = !_obscurePin),
                ),
                const SizedBox(height: 16),

                // Confirm PIN
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

                // Recover button
                SizedBox(
                  width: double.infinity,
                  child: FilledButton.icon(
                    onPressed: _isRecovering ? null : _recoverProfile,
                    icon: _isRecovering
                        ? const SizedBox(
                            width: 20,
                            height: 20,
                            child: CircularProgressIndicator(
                                strokeWidth: 2, color: Colors.black))
                        : const Icon(Icons.restore_rounded),
                    label: Text(
                        _isRecovering ? 'Recovering...' : 'Recover Profile'),
                    style: FilledButton.styleFrom(
                      backgroundColor: RoyalTheme.gold,
                      foregroundColor: Colors.black,
                      padding: const EdgeInsets.symmetric(vertical: 18),
                      shape: RoundedRectangleBorder(
                          borderRadius: BorderRadius.circular(12)),
                      textStyle: const TextStyle(
                          fontWeight: FontWeight.bold, fontSize: 16),
                    ),
                  ),
                ),
              ],
            ),
          ),
        ),
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
            fontSize: 24,
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
}
