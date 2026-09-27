import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_animate/flutter_animate.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:models/models.dart';
import 'package:provider/provider.dart';
import 'package:royal_app/l10n/generated/app_localizations.dart';
import '../main.dart';
import '../theme.dart';
import 'registration_screen.dart';
import 'profile_selection_screen.dart';

/// Onboarding screen - entry point after bridge setup
/// Options: Create New Profile, Recover from Seed, Select Existing Profile
class OnboardingScreen extends StatefulWidget {
  final VoidCallback onProfileActivated;

  const OnboardingScreen({
    super.key,
    required this.onProfileActivated,
  });

  @override
  State<OnboardingScreen> createState() => _OnboardingScreenState();
}

class _OnboardingScreenState extends State<OnboardingScreen>
    with TickerProviderStateMixin {
  late AppLocalizations _l10n;
  OnboardingState _state = OnboardingState.selecting;
  List<ProfileInfo> _profiles = [];
  String? _activeProfileId;
  String? _errorMessage;
  bool _isLoading = false;

  // Animation controllers
  late AnimationController _fadeCtrl;
  late AnimationController _slideCtrl;
  late Animation<double> _fadeAnim;
  late Animation<Offset> _slideAnim;

  // PIN entry state
  ProfileInfo? _selectedProfile;
  int _pinErrorCount = 0;
  DateTime? _lockoutUntil;
  final TextEditingController _pinCtrl = TextEditingController();
  bool _pinObscure = true;

  @override
  void initState() {
    super.initState();
    _fadeCtrl = AnimationController(
        duration: const Duration(milliseconds: 500), vsync: this);
    _slideCtrl = AnimationController(
        duration: const Duration(milliseconds: 400), vsync: this);
    _fadeAnim = Tween<double>(begin: 0.0, end: 1.0)
        .animate(CurvedAnimation(parent: _fadeCtrl, curve: Curves.easeOut));
    _slideAnim = Tween<Offset>(begin: const Offset(0, 0.2), end: Offset.zero)
        .animate(CurvedAnimation(parent: _slideCtrl, curve: Curves.easeOut));
    _fadeCtrl.forward();
    _slideCtrl.forward();
    _loadProfiles();
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _l10n = AppLocalizations.of(context)!;
  }

  @override
  void dispose() {
    _fadeCtrl.dispose();
    _slideCtrl.dispose();
    _pinCtrl.dispose();
    super.dispose();
  }

  Future<void> _loadProfiles() async {
    try {
      final prefs = await SharedPreferences.getInstance();
      final profileIds = prefs.getStringList('profile_ids') ?? [];
      _activeProfileId = prefs.getString('active_profile');

      _profiles = [];
      for (final id in profileIds) {
        final name = prefs.getString('${id}_name') ?? id;
        final created = prefs.getString('${id}_created');
        final panicMode = prefs.getBool('${id}_panic_mode') ?? false;
        _profiles.add(ProfileInfo(
          id: id,
          name: name,
          created: created != null ? DateTime.parse(created) : DateTime.now(),
          panicMode: panicMode,
        ));
      }

      if (mounted) setState(() {});
    } catch (e) {
      if (mounted) {
        setState(() => _errorMessage = 'Failed to load profiles: $e');
      }
    }
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

          SafeArea(
            child: FadeTransition(
              opacity: _fadeAnim,
              child: SlideTransition(
                position: _slideAnim,
                child: Center(
                  child: SingleChildScrollView(
                    padding: const EdgeInsets.all(32),
                    child: ConstrainedBox(
                      constraints: const BoxConstraints(maxWidth: 560),
                      child: Column(
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          // Logo/Icon
                          Container(
                            width: 120,
                            height: 120,
                            decoration: BoxDecoration(
                              shape: BoxShape.circle,
                              gradient: RadialGradient(
                                colors: [
                                  RoyalTheme.gold.withAlpha(60),
                                  RoyalTheme.deepNavy,
                                ],
                              ),
                              border: Border.all(
                                color: RoyalTheme.gold.withAlpha(120),
                                width: 2,
                              ),
                              boxShadow: [
                                BoxShadow(
                                  color: RoyalTheme.gold.withAlpha(30),
                                  blurRadius: 40,
                                  spreadRadius: 5,
                                ),
                              ],
                            ),
                            child: Center(
                              child: const Icon(
                                Icons.castle_rounded,
                                size: 64,
                                color: RoyalTheme.gold,
                              ),
                            ),
                          )
                              .animate()
                              .fadeIn(duration: 600.ms)
                              .scale(delay: 200.ms),

                          const SizedBox(height: 32),

                          // Title
                          Text(
                            _l10n.onboardingTitle,
                            style: theme.textTheme.headlineLarge?.copyWith(
                              fontWeight: FontWeight.w700,
                              color: Colors.white,
                            ),
                            textAlign: TextAlign.center,
                          )
                              .animate()
                              .fadeIn(duration: 400.ms, delay: 300.ms)
                              .slideY(begin: 0.2),

                          const SizedBox(height: 12),

                          // Subtitle
                          Text(
                            _state == OnboardingState.selecting
                                ? 'Choose how to proceed'
                                : _getSubtitleForState(),
                            style: theme.textTheme.bodyLarge?.copyWith(
                              color: RoyalTheme.silver,
                            ),
                            textAlign: TextAlign.center,
                          )
                              .animate()
                              .fadeIn(duration: 400.ms, delay: 400.ms)
                              .slideY(begin: 0.2),

                          const SizedBox(height: 48),

                          // Error message
                          if (_errorMessage != null)
                            Container(
                              width: double.infinity,
                              padding: const EdgeInsets.all(20),
                              margin: const EdgeInsets.only(bottom: 24),
                              decoration: BoxDecoration(
                                color: RoyalTheme.red.withAlpha(30),
                                borderRadius: BorderRadius.circular(12),
                                border: Border.all(
                                    color: RoyalTheme.red.withAlpha(80)),
                              ),
                              child: Row(
                                children: [
                                  const Icon(Icons.error_outline_rounded,
                                      color: RoyalTheme.red, size: 24),
                                  const SizedBox(width: 12),
                                  Expanded(
                                    child: Text(
                                      _errorMessage!,
                                      style: theme.textTheme.bodyMedium
                                          ?.copyWith(color: RoyalTheme.red),
                                    ),
                                  ),
                                ],
                              ),
                            ).animate().shake().fadeIn(),

                          // State-based content
                          _buildStateContent(),

                          const SizedBox(height: 32),

                          // Footer
                          Text(
                            'Saint Mary Liberty Island • Royal Control',
                            style: theme.textTheme.bodySmall?.copyWith(
                              color: RoyalTheme.silver.withAlpha(100),
                            ),
                            textAlign: TextAlign.center,
                          ),
                        ],
                      ),
                    ),
                  ),
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }

  String _getSubtitleForState() {
    switch (_state) {
      case OnboardingState.selecting:
        return '';
      case OnboardingState.creating:
        return 'Creating new BIP39 profile...';
      case OnboardingState.recovering:
        return 'Recovering profile from seed phrase...';
      case OnboardingState.selectingProfile:
        return 'Select a profile to activate';
      case OnboardingState.enteringPin:
        return 'Enter your 8-digit PIN';
    }
  }

  Widget _buildStateContent() {
    switch (_state) {
      case OnboardingState.selecting:
        return _buildSelectionOptions();
      case OnboardingState.creating:
        return RegistrationScreen(
            onRegistrationComplete: _onRegistrationComplete);
      case OnboardingState.recovering:
        return _buildSeedRecoveryScreen();
      case OnboardingState.selectingProfile:
        return _buildProfileList();
      case OnboardingState.enteringPin:
        return _buildPinEntryScreen();
    }
  }

  Widget _buildSelectionOptions() {
    return Column(
      children: [
        // Create New Profile
        _OnboardingOptionCard(
          icon: Icons.add_circle_outline_rounded,
          title: _l10n.createNewProfile,
          subtitle: 'Generate new BIP39 seed phrase and set PIN',
          color: RoyalTheme.green,
          onTap: () => setState(() => _state = OnboardingState.creating),
        ),

        const SizedBox(height: 16),

        // Recover from Seed
        _OnboardingOptionCard(
          icon: Icons.restore_rounded,
          title: _l10n.recoverFromSeed,
          subtitle: 'Restore existing profile using 12/24 word seed',
          color: RoyalTheme.accent,
          onTap: () => setState(() => _state = OnboardingState.recovering),
        ),

        // Select Existing Profile (only show if profiles exist)
        if (_profiles.isNotEmpty) ...[
          const SizedBox(height: 16),
          _OnboardingOptionCard(
            icon: Icons.person_outline_rounded,
            title: _l10n.selectExistingProfile,
            subtitle:
                '${_profiles.length} profile${_profiles.length > 1 ? 's' : ''} available',
            color: RoyalTheme.gold,
            onTap: () =>
                setState(() => _state = OnboardingState.selectingProfile),
          ),
        ],
      ],
    );
  }

  Widget _buildProfileList() {
    final theme = Theme.of(context);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            Text(
              _l10n.profilesList,
              style: theme.textTheme.titleMedium?.copyWith(
                color: RoyalTheme.gold,
                fontWeight: FontWeight.w600,
              ),
            ),
            const Spacer(),
            IconButton(
              onPressed: () =>
                  setState(() => _state = OnboardingState.selecting),
              icon: const Icon(Icons.arrow_back_rounded, color: RoyalTheme.silver),
              tooltip: _l10n.back,
            ),
          ],
        ),
        const SizedBox(height: 16),
        ..._profiles.map((profile) => _ProfileListTile(
              profile: profile,
              isActive: profile.id == _activeProfileId,
              onTap: () => _activateProfile(profile),
            )),
      ],
    );
  }

  void _activateProfile(ProfileInfo profile) {
    setState(() {
      _state = OnboardingState.enteringPin;
      _selectedProfile = profile;
    });
  }

  Widget _buildPinEntryScreen() {
    final theme = Theme.of(context);
    final profile = _selectedProfile!;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            IconButton(
              onPressed: () =>
                  setState(() => _state = OnboardingState.selectingProfile),
              icon: const Icon(Icons.arrow_back_rounded, color: RoyalTheme.silver),
              tooltip: _l10n.back,
            ),
            Text(
              profile.name,
              style: theme.textTheme.titleMedium?.copyWith(
                color: RoyalTheme.gold,
                fontWeight: FontWeight.w600,
              ),
            ),
          ],
        ),
        const SizedBox(height: 6),
        Text(
          _l10n.enterPin,
          style: theme.textTheme.bodyMedium?.copyWith(color: RoyalTheme.silver),
        ),
        const SizedBox(height: 32),

        // PIN input
        _PinEntryField(
          controller: _pinCtrl,
          obscure: _pinObscure,
          onToggleVisibility: () => setState(() => _pinObscure = !_pinObscure),
          onSubmitted: _verifyPin,
          errorMessage: _errorMessage,
          isLockedOut:
              _lockoutUntil != null && DateTime.now().isBefore(_lockoutUntil!),
          lockoutUntil: _lockoutUntil,
        ),

        const SizedBox(height: 24),

        // Panic mode warning
        if (profile.panicMode)
          Container(
            width: double.infinity,
            padding: const EdgeInsets.all(16),
            decoration: BoxDecoration(
              color: RoyalTheme.red.withAlpha(30),
              borderRadius: BorderRadius.circular(12),
              border: Border.all(color: RoyalTheme.red.withAlpha(80)),
            ),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    const Icon(Icons.warning_amber_rounded,
                        color: RoyalTheme.red, size: 24),
                    const SizedBox(width: 12),
                    Text(
                      _l10n.panicMode,
                      style: theme.textTheme.titleMedium?.copyWith(
                          color: RoyalTheme.red, fontWeight: FontWeight.w600),
                    ),
                  ],
                ),
                const SizedBox(height: 8),
                Text(
                  _l10n.panicMessage,
                  style:
                      theme.textTheme.bodyMedium?.copyWith(color: Colors.white),
                ),
              ],
            ),
          ),

        // Action buttons
        const SizedBox(height: 24),
        Row(
          children: [
            Expanded(
              child: OutlinedButton.icon(
                onPressed: () =>
                    setState(() => _state = OnboardingState.selectingProfile),
                icon: const Icon(Icons.cancel_rounded),
                label: Text(_l10n.cancel),
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
                onPressed: _isLoading ||
                        (_lockoutUntil != null &&
                            DateTime.now().isBefore(_lockoutUntil!))
                    ? null
                    : () => _verifyPin(_pinCtrl.text),
                icon: _isLoading
                    ? const SizedBox(
                        width: 20,
                        height: 20,
                        child: CircularProgressIndicator(strokeWidth: 2))
                    : const Icon(Icons.lock_open_rounded),
                label: Text(_l10n.unlock),
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
    );
  }

  Future<void> _verifyPin(String pin) async {
    if (pin.length != 8 || !RegExp(r'^\d{8}$').hasMatch(pin)) {
      setState(() => _errorMessage = _l10n.invalidPinLength);
      return;
    }

    setState(() {
      _isLoading = true;
      _errorMessage = null;
    });

    try {
      final prefs = await SharedPreferences.getInstance();
      final identityService = await SecureIdentityService.instance;

      final storedHash = prefs.getString('${_selectedProfile!.id}_pin_hash');
      final encrypted = prefs.getString('${_selectedProfile!.id}_encrypted');

      if (storedHash == null || encrypted == null) {
        throw Exception('Profile data corrupted');
      }

      // Verify PIN hash
      final pinHash = await identityService.hashPin(pin);
      if (pinHash != storedHash) {
        throw Exception('Invalid PIN');
      }

      // Decrypt identity
      final identity = await identityService.decryptIdentity(encrypted, pin);

      // Set active profile
      await prefs.setString('active_profile', _selectedProfile!.id);

      // Set identity in app state
      if (!mounted) return;
      final appState = context.read<AppState>();
      appState.setIdentity(identity);
      appState.setUnlocked(true);

      widget.onProfileActivated();
    } catch (e) {
      setState(() {
        _errorMessage = 'PIN verification failed: $e';
        _pinErrorCount++;
        _pinCtrl.clear();

        // Exponential backoff: 30s, 60s, 120s, 240s...
        if (_pinErrorCount >= 3) {
          final delaySeconds = 30 * (1 << (_pinErrorCount - 3));
          _lockoutUntil = DateTime.now().add(Duration(seconds: delaySeconds));
        }
      });
    } finally {
      if (mounted) setState(() => _isLoading = false);
    }
  }

  Widget _buildSeedRecoveryScreen() {
    // Use the existing SeedRecoveryScreen from profile_selection_screen.dart
    return SeedRecoveryScreen(
      onRecoveryComplete: _onRegistrationComplete,
      onBack: () => setState(() => _state = OnboardingState.selecting),
    );
  }

  void _onRegistrationComplete() {
    _loadProfiles().then((_) {
      if (mounted) {
        setState(() => _state = OnboardingState.enteringPin);
      }
    });
  }
}

enum OnboardingState {
  selecting,
  creating,
  recovering,
  selectingProfile,
  enteringPin,
}

class _OnboardingOptionCard extends StatelessWidget {
  final IconData icon;
  final String title;
  final String subtitle;
  final Color color;
  final VoidCallback onTap;

  const _OnboardingOptionCard({
    required this.icon,
    required this.title,
    required this.subtitle,
    required this.color,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Material(
      color: Colors.transparent,
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(16),
        child: Container(
          width: double.infinity,
          padding: const EdgeInsets.all(24),
          decoration: BoxDecoration(
            color: RoyalTheme.darkCard,
            borderRadius: BorderRadius.circular(16),
            border: Border.all(color: color.withAlpha(80), width: 1.5),
          ),
          child: Row(
            children: [
              Container(
                width: 56,
                height: 56,
                decoration: BoxDecoration(
                  color: color.withAlpha(30),
                  shape: BoxShape.circle,
                  border: Border.all(color: color, width: 2),
                ),
                child: Icon(icon, color: color, size: 28),
              ),
              const SizedBox(width: 20),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      title,
                      style: theme.textTheme.titleLarge?.copyWith(
                        fontWeight: FontWeight.w700,
                        color: Colors.white,
                      ),
                    ),
                    const SizedBox(height: 6),
                    Text(
                      subtitle,
                      style: theme.textTheme.bodyMedium?.copyWith(
                        color: RoyalTheme.silver,
                      ),
                    ),
                  ],
                ),
              ),
              Icon(Icons.arrow_forward_ios_rounded, color: color, size: 20),
            ],
          ),
        ),
      ),
    );
  }
}

class _ProfileListTile extends StatelessWidget {
  final ProfileInfo profile;
  final bool isActive;
  final VoidCallback onTap;

  const _ProfileListTile({
    required this.profile,
    required this.isActive,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Material(
      color: Colors.transparent,
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(12),
        child: Container(
          margin: const EdgeInsets.only(bottom: 12),
          padding: const EdgeInsets.all(16),
          decoration: BoxDecoration(
            color:
                isActive ? RoyalTheme.gold.withAlpha(20) : RoyalTheme.darkCard,
            borderRadius: BorderRadius.circular(12),
            border: Border.all(
              color: isActive ? RoyalTheme.gold : Colors.grey[800]!,
              width: isActive ? 2 : 1,
            ),
          ),
          child: Row(
            children: [
              Container(
                width: 48,
                height: 48,
                decoration: BoxDecoration(
                  color: isActive
                      ? RoyalTheme.gold.withAlpha(30)
                      : RoyalTheme.darkCard,
                  shape: BoxShape.circle,
                  border: Border.all(
                    color: isActive ? RoyalTheme.gold : Colors.grey[700]!,
                    width: 1.5,
                  ),
                ),
                child: Icon(
                  isActive
                      ? Icons.radio_button_checked_rounded
                      : Icons.radio_button_unchecked_rounded,
                  color: isActive ? RoyalTheme.gold : RoyalTheme.silver,
                  size: 24,
                ),
              ),
              const SizedBox(width: 16),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      profile.name,
                      style: theme.textTheme.bodyLarge?.copyWith(
                        fontWeight: FontWeight.w600,
                        color: Colors.white,
                      ),
                    ),
                    const SizedBox(height: 2),
                    Text(
                      'Created: ${profile.created.day.toString().padLeft(2, '0')}.${profile.created.month.toString().padLeft(2, '0')}.${profile.created.year}',
                      style: theme.textTheme.bodySmall?.copyWith(
                        color: RoyalTheme.silver.withAlpha(150),
                      ),
                    ),
                    if (profile.panicMode)
                      Container(
                        margin: const EdgeInsets.only(top: 4),
                        padding: const EdgeInsets.symmetric(
                            horizontal: 8, vertical: 2),
                        decoration: BoxDecoration(
                          color: RoyalTheme.red.withAlpha(30),
                          borderRadius: BorderRadius.circular(8),
                          border:
                              Border.all(color: RoyalTheme.red.withAlpha(80)),
                        ),
                        child: Text(
                          'PANIC MODE',
                          style: theme.textTheme.labelSmall?.copyWith(
                            color: RoyalTheme.red,
                            fontWeight: FontWeight.w600,
                          ),
                        ),
                      ),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _PinEntryField extends StatelessWidget {
  final TextEditingController controller;
  final bool obscure;
  final VoidCallback onToggleVisibility;
  final Function(String) onSubmitted;
  final String? errorMessage;
  final bool isLockedOut;
  final DateTime? lockoutUntil;

  const _PinEntryField({
    required this.controller,
    required this.obscure,
    required this.onToggleVisibility,
    required this.onSubmitted,
    this.errorMessage,
    this.isLockedOut = false,
    this.lockoutUntil,
  });

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Column(
      children: [
        TextField(
          controller: controller,
          obscureText: obscure,
          maxLength: 8,
          keyboardType: TextInputType.number,
          textAlign: TextAlign.center,
          style: theme.textTheme.headlineMedium?.copyWith(
            letterSpacing: 8,
            fontFeatures: const [FontFeature.tabularFigures()],
            color: Colors.white,
          ),
          decoration: InputDecoration(
            hintText: '• • • • • • • •',
            hintStyle: theme.textTheme.headlineMedium?.copyWith(
              letterSpacing: 8,
              fontFeatures: const [FontFeature.tabularFigures()],
              color: RoyalTheme.silver.withAlpha(80),
            ),
            counterText: '',
            suffixIcon: IconButton(
              onPressed: onToggleVisibility,
              icon: Icon(
                obscure
                    ? Icons.visibility_off_rounded
                    : Icons.visibility_rounded,
                color: RoyalTheme.silver,
              ),
            ),
            border: OutlineInputBorder(
              borderRadius: BorderRadius.circular(16),
              borderSide: BorderSide(
                color: errorMessage != null ? RoyalTheme.red : RoyalTheme.gold,
                width: 2,
              ),
            ),
            enabledBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(16),
              borderSide: BorderSide(
                color:
                    errorMessage != null ? RoyalTheme.red : Colors.grey[700]!,
                width: 1.5,
              ),
            ),
            focusedBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(16),
              borderSide: BorderSide(
                color: errorMessage != null ? RoyalTheme.red : RoyalTheme.gold,
                width: 2,
              ),
            ),
            errorBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(16),
              borderSide: const BorderSide(color: RoyalTheme.red, width: 2),
            ),
            filled: true,
            fillColor: const Color(0xFF21262D),
            errorText: errorMessage,
            errorStyle:
                theme.textTheme.bodySmall?.copyWith(color: RoyalTheme.red),
          ),
          onSubmitted: onSubmitted,
          enabled: !isLockedOut,
        ),
        if (isLockedOut && lockoutUntil != null)
          Padding(
            padding: const EdgeInsets.only(top: 12),
            child: Text(
              'Locked for ${_formatDuration(lockoutUntil!.difference(DateTime.now()))}',
              style: theme.textTheme.bodyMedium
                  ?.copyWith(color: RoyalTheme.orange),
            ),
          ),
      ],
    );
  }

  String _formatDuration(Duration duration) {
    final minutes = duration.inMinutes;
    final seconds = duration.inSeconds % 60;
    if (minutes > 0) {
      return '${minutes}m ${seconds}s';
    }
    return '${seconds}s';
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
