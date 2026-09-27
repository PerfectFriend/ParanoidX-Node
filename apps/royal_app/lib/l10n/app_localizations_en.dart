// ignore: unused_import
import 'package:intl/intl.dart' as intl;
import 'app_localizations.dart';

// ignore_for_file: type=lint

/// The translations for English (`en`).
class AppLocalizationsEn extends AppLocalizations {
  AppLocalizationsEn([String locale = 'en']) : super(locale);

  @override
  String get appTitle => 'Royal Control';

  @override
  String get bootMessage => 'Booting Royal Control';

  @override
  String get networkBridgeSetup => 'Network Bridge Setup';

  @override
  String get testingNetworkEnvironment => 'Testing network environment properties...';

  @override
  String get connectingProtocols => 'Connecting configured protocols...';

  @override
  String testingProtocol(Object protocol) {
    return 'Testing $protocol...';
  }

  @override
  String protocolSuccess(Object protocol) {
    return '$protocol - OK';
  }

  @override
  String protocolFailed(Object protocol) {
    return '$protocol - Failed';
  }

  @override
  String get selectingOptimalSequence => 'Selecting optimal protocol sequence...';

  @override
  String get preferredSequence => 'Preferred sequence: v2ray → tor';

  @override
  String get testingSimplexNode => 'Testing Simplex node connectivity...';

  @override
  String get simplexNodeAvailable => 'Simplex node available';

  @override
  String get simplexNodeUnavailable => 'Simplex node unavailable';

  @override
  String get bridgeConfiguration => 'Bridge Configuration';

  @override
  String get configureProtocols => 'Configure Protocols';

  @override
  String get loadConfigFromFile => 'Load Config from File';

  @override
  String get saveConfiguration => 'Save Configuration';

  @override
  String get startBridge => 'Start Bridge';

  @override
  String get bridgeStatus => 'Bridge Status';

  @override
  String get protocolStatus => 'Protocol Status';

  @override
  String get serviceStatus => 'Service Status';

  @override
  String get onboardingTitle => 'Welcome to Royal Control';

  @override
  String get createNewProfile => 'Create New Profile';

  @override
  String get recoverFromSeed => 'Recover from Seed Phrase';

  @override
  String get selectExistingProfile => 'Select Existing Profile';

  @override
  String get profilesList => 'Available Profiles';

  @override
  String get enterPin => 'Enter PIN';

  @override
  String get pinHint => '8-digit PIN';

  @override
  String get showPin => 'Show PIN';

  @override
  String get hidePin => 'Hide PIN';

  @override
  String get unlock => 'Unlock';

  @override
  String get welcomeBack => 'Welcome back';

  @override
  String get dashboard => 'Dashboard';

  @override
  String get settings => 'Settings';

  @override
  String get panicMode => 'Panic Mode Activated';

  @override
  String get panicMessage => 'PIN access disabled. Use seed recovery to regain access.';

  @override
  String get seedWarning => 'IMPORTANT: Write down your seed phrase and store it securely. This is the only way to recover your profile.';

  @override
  String get verifySeedWords => 'Verify your seed phrase by selecting the 3 requested words:';

  @override
  String get confirmPin => 'Confirm PIN';

  @override
  String get pinMismatch => 'PINs do not match';

  @override
  String get invalidPinLength => 'PIN must be 8 digits';

  @override
  String get registrationComplete => 'Registration complete';

  @override
  String get language => 'Language';

  @override
  String get english => 'English';

  @override
  String get systemLanguage => 'System Language';

  @override
  String get loading => 'Loading...';

  @override
  String get error => 'Error';

  @override
  String get success => 'Success';

  @override
  String get cancel => 'Cancel';

  @override
  String get continueAction => 'Continue';

  @override
  String get back => 'Back';

  @override
  String get next => 'Next';

  @override
  String get russianDetected => 'Russian detected';

  @override
  String get englishDetected => 'English detected';
}
