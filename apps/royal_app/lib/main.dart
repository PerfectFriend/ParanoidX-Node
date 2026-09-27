import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:models/models.dart';
import 'package:royal_app/l10n/generated/app_localizations.dart';
import 'dart:async';
import 'theme.dart';
import 'services/royal_api_service.dart';
import 'screens/dashboard_screen.dart';
import 'screens/ai_office_screen.dart';
import 'screens/treasury_screen.dart';
import 'screens/communications_screen.dart';
import 'screens/dc_cloud_screen.dart';
import 'screens/governance_screen.dart';
import 'screens/system_screen.dart';
import 'screens/settings_screen.dart';
import 'screens/lock_screen.dart';
import 'screens/registration_screen.dart';
import 'screens/profile_selection_screen.dart';
import 'screens/boot_screen.dart';
import 'screens/bridge_setup_screen.dart';
import 'screens/onboarding_screen.dart';

void main() {
  runApp(const RoyalApp());
}

/// RoyalApp manages the root MaterialApp widget for the Royal Control client.
class RoyalApp extends StatelessWidget {
  const RoyalApp({super.key});

  @override
  Widget build(BuildContext context) {
    return ChangeNotifierProvider(
      create: (_) => AppState(),
      child: MaterialApp(
        title: 'Royal Control',
        theme: RoyalTheme.dark,
        debugShowCheckedModeBanner: false,
        localizationsDelegates: const [
          AppLocalizations.delegate,
          GlobalMaterialLocalizations.delegate,
          GlobalWidgetsLocalizations.delegate,
          GlobalCupertinoLocalizations.delegate,
        ],
        supportedLocales: const [
          Locale('en'),
          Locale('ru'),
        ],
        home: const AuthGate(),
      ),
    );
  }
}

/// AuthGate decides which screen to show based on profile state and unlock status.
/// Flow:
/// 1. BootScreen -> BridgeSetupScreen -> OnboardingScreen
/// 2. No profiles -> RegistrationScreen (BIP39 setup)
/// 3. Profiles exist but unlocked -> RoyalShell (Dashboard)
/// 4. Profiles exist and locked -> ProfileSelectionScreen (choose profile + PIN)
/// 5. Recover from seed -> SeedRecoveryScreen
class AuthGate extends StatefulWidget {
  const AuthGate({super.key});

  @override
  State<AuthGate> createState() => _AuthGateState();
}

class _AuthGateState extends State<AuthGate> {
  AuthFlowState _flowState = AuthFlowState.boot;
  List<ProfileInfo> _profiles = [];
  String? _activeProfileId;
  String? _errorMessage;

  @override
  void initState() {
    super.initState();
    // Start with boot screen - the new flow
    _flowState = AuthFlowState.boot;
  }

  void _onBootComplete() {
    setState(() => _flowState = AuthFlowState.bridgeSetup);
  }

  void _onBridgeReady() {
    setState(() => _flowState = AuthFlowState.onboarding);
  }

  void _onOnboardingComplete() {
    _checkProfiles();
  }

  Future<void> _checkProfiles() async {
    try {
      final prefs = await SharedPreferences.getInstance();
      final profileIds = prefs.getStringList('profile_ids') ?? [];
      _activeProfileId = prefs.getString('active_profile');

      if (profileIds.isEmpty) {
        setState(() => _flowState = AuthFlowState.registration);
        return;
      }

      // Load profile info for selection
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

      if (_activeProfileId != null && !prefs.getBool('${_activeProfileId}_panic_mode')!) {
        // Has active profile not in panic mode - show lock screen for that profile
        setState(() => _flowState = AuthFlowState.lockScreen);
      } else {
        // Show profile selection
        setState(() => _flowState = AuthFlowState.profileSelection);
      }
    } catch (e) {
      setState(() {
        _flowState = AuthFlowState.error;
        _errorMessage = 'Failed to load profiles: $e';
      });
    }
  }

  void _onRegistrationComplete() {
    _checkProfiles();
  }

  void _onUnlockSuccess() {
    final state = context.read<AppState>();
    state.setUnlocked(true);
    state.startSSE();
    // Transition to unlocked state - AuthGate.build will show RoyalShell
    setState(() => _flowState = AuthFlowState.unlocked);
  }

  void _onProfileSelected(String profileId) {
    final prefs = SharedPreferences.getInstance();
    prefs.then((p) => p.setString('active_profile', profileId));
    setState(() {
      _activeProfileId = profileId;
      _flowState = AuthFlowState.lockScreen;
    });
  }

  void _onRecoverFromSeed() {
    setState(() => _flowState = AuthFlowState.seedRecovery);
  }

  void _onBackToProfileSelection() {
    setState(() => _flowState = AuthFlowState.profileSelection);
  }

  void _onProfileCreated() {
    _checkProfiles();
  }

  @override
  Widget build(BuildContext context) {
    final appState = context.watch<AppState>();

    // If unlocked, show the main app shell
    if (appState.unlocked) {
      return const RoyalShell();
    }

    switch (_flowState) {
      case AuthFlowState.boot:
        return BootScreen(onBootComplete: _onBootComplete);
      case AuthFlowState.bridgeSetup:
        return BridgeSetupScreen(onBridgeReady: _onBridgeReady);
      case AuthFlowState.onboarding:
        return OnboardingScreen(onProfileActivated: _onOnboardingComplete);
      case AuthFlowState.checking:
        return _buildLoadingScreen();
      case AuthFlowState.registration:
        return RegistrationScreen(onRegistrationComplete: _onRegistrationComplete);
      case AuthFlowState.profileSelection:
        return ProfileSelectionScreen(
          profiles: _profiles,
          onProfileSelected: _onProfileSelected,
          onRecoverFromSeed: _onRecoverFromSeed,
          onCreateNewProfile: _onProfileCreated,
        );
      case AuthFlowState.lockScreen:
        if (_activeProfileId == null) {
          return _buildErrorScreen('No active profile selected');
        }
        return LockScreen(
          profileId: _activeProfileId!,
          onUnlocked: _onUnlockSuccess,
          onBackToProfiles: _onBackToProfileSelection,
          onRecoverFromSeed: _onRecoverFromSeed,
        );
      case AuthFlowState.seedRecovery:
        return SeedRecoveryScreen(
          onRecoveryComplete: _onRegistrationComplete,
          onBack: _onBackToProfileSelection,
        );
      case AuthFlowState.error:
        return _buildErrorScreen(_errorMessage ?? 'Unknown error');
      case AuthFlowState.unlocked:
        return const RoyalShell();
    }
  }

  Widget _buildLoadingScreen() {
    return Scaffold(
      backgroundColor: RoyalTheme.darkScaffold,
      body: Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Container(
              width: 80,
              height: 80,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                border: Border.all(color: RoyalTheme.gold, width: 2),
                boxShadow: [BoxShadow(color: RoyalTheme.gold.withAlpha(40), blurRadius: 16)],
              ),
              child: const Center(child: CircularProgressIndicator(color: RoyalTheme.gold, strokeWidth: 3)),
            ),
            const SizedBox(height: 24),
            const Text('Loading Royal Control...', style: TextStyle(color: Colors.white, fontSize: 16)),
          ],
        ),
      ),
    );
  }

  Widget _buildErrorScreen(String message) {
    return Scaffold(
      backgroundColor: RoyalTheme.darkScaffold,
      body: Center(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(Icons.error_outline, color: RoyalTheme.red, size: 64),
              const SizedBox(height: 24),
              const Text('Error', style: TextStyle(color: Colors.white, fontSize: 24, fontWeight: FontWeight.bold)),
              const SizedBox(height: 12),
              Text(message, style: TextStyle(color: Colors.grey[400], fontSize: 16), textAlign: TextAlign.center),
              const SizedBox(height: 24),
              FilledButton(
                style: FilledButton.styleFrom(backgroundColor: RoyalTheme.gold, foregroundColor: Colors.black),
                onPressed: _checkProfiles,
                child: const Text('Retry'),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

enum AuthFlowState {
  boot,
  bridgeSetup,
  onboarding,
  checking,
  registration,
  profileSelection,
  lockScreen,
  seedRecovery,
  error,
  unlocked,
}

/// AppState manages application-wide state management for the Royal client.
class AppState extends ChangeNotifier {
  final RoyalApiService api = RoyalApiService();
  int selectedIndex = 0;
  bool isOffline = false;
  bool unlocked = false;
  String? activeProfileId;
  Map<String, dynamic>? liveTelemetry;
  StreamSubscription<dynamic>? _sseSub;
  Identity? _identity;

  AppState();

  Identity? get identity => _identity;

  void setIdentity(Identity identity) {
    _identity = identity;
    notifyListeners();
  }

  void startSSE() {
    if (_sseSub != null) return; // Already started
    try {
      _sseSub?.cancel();
      _sseSub = api.sseEvents().listen((data) {
        liveTelemetry = data;
        notifyListeners();
      }, onError: (_) {});
    } catch (_) {}
  }

  void stopSSE() {
    _sseSub?.cancel();
    _sseSub = null;
  }

  void setIndex(int i) {
    selectedIndex = i;
    notifyListeners();
  }

  void setOffline(bool v) {
    isOffline = v;
    notifyListeners();
  }

  void setUnlocked(bool v) {
    unlocked = v;
    notifyListeners();
  }

  void setActiveProfile(String profileId) {
    activeProfileId = profileId;
    notifyListeners();
  }

  @override
  void dispose() {
    _sseSub?.cancel();
    api.dispose();
    super.dispose();
  }
}

/// RoyalShell manages the main shell with NavigationRail-based screen routing.
class RoyalShell extends StatefulWidget {
  const RoyalShell({super.key});

  @override
  State<RoyalShell> createState() => _RoyalShellState();
}

class _RoyalShellState extends State<RoyalShell> {
  final List<Widget> _screens = [
    const DashboardScreen(),
    const AIOfficeScreen(),
    const TreasuryScreen(),
    const CommunicationsScreen(),
    const DCCloudScreen(),
    const GovernanceScreen(),
    const SystemScreen(),
    const SettingsScreen(),
  ];

  @override
  Widget build(BuildContext context) {
    final state = context.watch<AppState>();
    return LayoutBuilder(
      builder: (context, constraints) {
        final isWide = constraints.maxWidth > 900;
        return Scaffold(
          body: Row(
            children: [
              if (isWide)
                NavigationRail(
                  selectedIndex: state.selectedIndex,
                  onDestinationSelected: state.setIndex,
                  labelType: NavigationRailLabelType.all,
                  backgroundColor: RoyalTheme.darkCard,
                  indicatorColor: RoyalTheme.gold.withAlpha(40),
                  leading: Padding(
                    padding: const EdgeInsets.symmetric(vertical: 12),
                    child: Center(
                      child: Container(
                        width: 48,
                        height: 48,
                        decoration: BoxDecoration(
                          shape: BoxShape.circle,
                          border: Border.all(color: RoyalTheme.gold, width: 2),
                          boxShadow: [BoxShadow(color: RoyalTheme.gold.withAlpha(40), blurRadius: 8)],
                        ),
                        child: const Center(
                          child: Text('♚', style: TextStyle(fontSize: 24, color: RoyalTheme.gold)),
                        ),
                      ),
                    ),
                  ),
                  destinations: const [
                    NavigationRailDestination(icon: Icon(Icons.dashboard), label: Text('Dashboard')),
                    NavigationRailDestination(icon: Icon(Icons.auto_awesome), label: Text('AI Office')),
                    NavigationRailDestination(icon: Icon(Icons.account_balance), label: Text('Treasury')),
                    NavigationRailDestination(icon: Icon(Icons.chat), label: Text('Comms')),
                    NavigationRailDestination(icon: Icon(Icons.cloud), label: Text('DC Cloud')),
                    NavigationRailDestination(icon: Icon(Icons.gavel), label: Text('Governance')),
                    NavigationRailDestination(icon: Icon(Icons.monitor_heart), label: Text('System')),
                    NavigationRailDestination(icon: Icon(Icons.settings), label: Text('Settings')),
                  ],
                ),
              if (!isWide)
                NavigationBar(
                  selectedIndex: state.selectedIndex,
                  onDestinationSelected: state.setIndex,
                  backgroundColor: RoyalTheme.darkCard,
                  indicatorColor: RoyalTheme.gold.withAlpha(40),
                  destinations: const [
                    NavigationDestination(icon: Icon(Icons.dashboard), label: 'Dashboard'),
                    NavigationDestination(icon: Icon(Icons.auto_awesome), label: 'AI'),
                    NavigationDestination(icon: Icon(Icons.account_balance), label: 'Treasury'),
                    NavigationDestination(icon: Icon(Icons.chat), label: 'Chat'),
                    NavigationDestination(icon: Icon(Icons.cloud), label: 'DC'),
                    NavigationDestination(icon: Icon(Icons.gavel), label: 'Gov'),
                    NavigationDestination(icon: Icon(Icons.monitor_heart), label: 'System'),
                    NavigationDestination(icon: Icon(Icons.settings), label: 'Settings'),
                  ],
                ),
              const VerticalDivider(width: 1, thickness: 1),
              Expanded(
                child: AnimatedSwitcher(
                  duration: const Duration(milliseconds: 300),
                  child: _screens[state.selectedIndex],
                ),
              ),
            ],
          ),
        );
      },
    );
  }
}