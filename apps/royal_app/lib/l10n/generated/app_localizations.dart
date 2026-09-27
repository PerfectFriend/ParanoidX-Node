import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:intl/intl.dart' as intl;

import 'app_localizations_en.dart';
import 'app_localizations_ru.dart';

// ignore_for_file: type=lint

/// Callers can lookup localized strings with an instance of AppLocalizations
/// returned by `AppLocalizations.of(context)`.
///
/// Applications need to include `AppLocalizations.delegate()` in their app's
/// `localizationDelegates` list, and the locales they support in the app's
/// `supportedLocales` list. For example:
///
/// ```dart
/// import 'l10n/app_localizations.dart';
///
/// return MaterialApp(
///   localizationsDelegates: AppLocalizations.localizationsDelegates,
///   supportedLocales: AppLocalizations.supportedLocales,
///   home: MyApplicationHome(),
/// );
/// ```
///
/// ## Update pubspec.yaml
///
/// Please make sure to update your pubspec.yaml to include the following
/// packages:
///
/// ```yaml
/// dependencies:
///   # Internationalization support.
///   flutter_localizations:
///     sdk: flutter
///   intl: any # Use the pinned version from flutter_localizations
///
///   # Rest of dependencies
/// ```
///
/// ## iOS Applications
///
/// iOS applications define key application metadata, including supported
/// locales, in an Info.plist file that is built into the application bundle.
/// To configure the locales supported by your app, you’ll need to edit this
/// file.
///
/// First, open your project’s ios/Runner.xcworkspace Xcode workspace file.
/// Then, in the Project Navigator, open the Info.plist file under the Runner
/// project’s Runner folder.
///
/// Next, select the Information Property List item, select Add Item from the
/// Editor menu, then select Localizations from the pop-up menu.
///
/// Select and expand the newly-created Localizations item then, for each
/// locale your application supports, add a new item and select the locale
/// you wish to add from the pop-up menu in the Value field. This list should
/// be consistent with the languages listed in the AppLocalizations.supportedLocales
/// property.
abstract class AppLocalizations {
  AppLocalizations(String locale) : localeName = intl.Intl.canonicalizedLocale(locale.toString());

  final String localeName;

  static AppLocalizations? of(BuildContext context) {
    return Localizations.of<AppLocalizations>(context, AppLocalizations);
  }

  static const LocalizationsDelegate<AppLocalizations> delegate = _AppLocalizationsDelegate();

  /// A list of this localizations delegate along with the default localizations
  /// delegates.
  ///
  /// Returns a list of localizations delegates containing this delegate along with
  /// GlobalMaterialLocalizations.delegate, GlobalCupertinoLocalizations.delegate,
  /// and GlobalWidgetsLocalizations.delegate.
  ///
  /// Additional delegates can be added by appending to this list in
  /// MaterialApp. This list does not have to be used at all if a custom list
  /// of delegates is preferred or required.
  static const List<LocalizationsDelegate<dynamic>> localizationsDelegates = <LocalizationsDelegate<dynamic>>[
    delegate,
    GlobalMaterialLocalizations.delegate,
    GlobalCupertinoLocalizations.delegate,
    GlobalWidgetsLocalizations.delegate,
  ];

  /// A list of this localizations delegate's supported locales.
  static const List<Locale> supportedLocales = <Locale>[
    Locale('en'),
    Locale('ru')
  ];

  /// No description provided for @appTitle.
  ///
  /// In en, this message translates to:
  /// **'Royal Control'**
  String get appTitle;

  /// No description provided for @bootMessage.
  ///
  /// In en, this message translates to:
  /// **'Booting Royal Control'**
  String get bootMessage;

  /// No description provided for @networkBridgeSetup.
  ///
  /// In en, this message translates to:
  /// **'Network Bridge Setup'**
  String get networkBridgeSetup;

  /// No description provided for @testingNetworkEnvironment.
  ///
  /// In en, this message translates to:
  /// **'Testing network environment properties...'**
  String get testingNetworkEnvironment;

  /// No description provided for @connectingProtocols.
  ///
  /// In en, this message translates to:
  /// **'Connecting configured protocols...'**
  String get connectingProtocols;

  /// No description provided for @testingProtocol.
  ///
  /// In en, this message translates to:
  /// **'Testing {protocol}...'**
  String testingProtocol(Object protocol);

  /// No description provided for @protocolSuccess.
  ///
  /// In en, this message translates to:
  /// **'{protocol} - OK'**
  String protocolSuccess(Object protocol);

  /// No description provided for @protocolFailed.
  ///
  /// In en, this message translates to:
  /// **'{protocol} - Failed'**
  String protocolFailed(Object protocol);

  /// No description provided for @selectingOptimalSequence.
  ///
  /// In en, this message translates to:
  /// **'Selecting optimal protocol sequence...'**
  String get selectingOptimalSequence;

  /// No description provided for @preferredSequence.
  ///
  /// In en, this message translates to:
  /// **'Preferred sequence: v2ray → tor'**
  String get preferredSequence;

  /// No description provided for @testingSimplexNode.
  ///
  /// In en, this message translates to:
  /// **'Testing Simplex node connectivity...'**
  String get testingSimplexNode;

  /// No description provided for @simplexNodeAvailable.
  ///
  /// In en, this message translates to:
  /// **'Simplex node available'**
  String get simplexNodeAvailable;

  /// No description provided for @simplexNodeUnavailable.
  ///
  /// In en, this message translates to:
  /// **'Simplex node unavailable'**
  String get simplexNodeUnavailable;

  /// No description provided for @bridgeConfiguration.
  ///
  /// In en, this message translates to:
  /// **'Bridge Configuration'**
  String get bridgeConfiguration;

  /// No description provided for @configureProtocols.
  ///
  /// In en, this message translates to:
  /// **'Configure Protocols'**
  String get configureProtocols;

  /// No description provided for @loadConfigFromFile.
  ///
  /// In en, this message translates to:
  /// **'Load Config from File'**
  String get loadConfigFromFile;

  /// No description provided for @saveConfiguration.
  ///
  /// In en, this message translates to:
  /// **'Save Configuration'**
  String get saveConfiguration;

  /// No description provided for @startBridge.
  ///
  /// In en, this message translates to:
  /// **'Start Bridge'**
  String get startBridge;

  /// No description provided for @bridgeStatus.
  ///
  /// In en, this message translates to:
  /// **'Bridge Status'**
  String get bridgeStatus;

  /// No description provided for @protocolStatus.
  ///
  /// In en, this message translates to:
  /// **'Protocol Status'**
  String get protocolStatus;

  /// No description provided for @serviceStatus.
  ///
  /// In en, this message translates to:
  /// **'Service Status'**
  String get serviceStatus;

  /// No description provided for @onboardingTitle.
  ///
  /// In en, this message translates to:
  /// **'Welcome to Royal Control'**
  String get onboardingTitle;

  /// No description provided for @createNewProfile.
  ///
  /// In en, this message translates to:
  /// **'Create New Profile'**
  String get createNewProfile;

  /// No description provided for @recoverFromSeed.
  ///
  /// In en, this message translates to:
  /// **'Recover from Seed Phrase'**
  String get recoverFromSeed;

  /// No description provided for @selectExistingProfile.
  ///
  /// In en, this message translates to:
  /// **'Select Existing Profile'**
  String get selectExistingProfile;

  /// No description provided for @profilesList.
  ///
  /// In en, this message translates to:
  /// **'Available Profiles'**
  String get profilesList;

  /// No description provided for @enterPin.
  ///
  /// In en, this message translates to:
  /// **'Enter PIN'**
  String get enterPin;

  /// No description provided for @pinHint.
  ///
  /// In en, this message translates to:
  /// **'8-digit PIN'**
  String get pinHint;

  /// No description provided for @showPin.
  ///
  /// In en, this message translates to:
  /// **'Show PIN'**
  String get showPin;

  /// No description provided for @hidePin.
  ///
  /// In en, this message translates to:
  /// **'Hide PIN'**
  String get hidePin;

  /// No description provided for @unlock.
  ///
  /// In en, this message translates to:
  /// **'Unlock'**
  String get unlock;

  /// No description provided for @welcomeBack.
  ///
  /// In en, this message translates to:
  /// **'Welcome back'**
  String get welcomeBack;

  /// No description provided for @dashboard.
  ///
  /// In en, this message translates to:
  /// **'Dashboard'**
  String get dashboard;

  /// No description provided for @settings.
  ///
  /// In en, this message translates to:
  /// **'Settings'**
  String get settings;

  /// No description provided for @panicMode.
  ///
  /// In en, this message translates to:
  /// **'Panic Mode Activated'**
  String get panicMode;

  /// No description provided for @panicMessage.
  ///
  /// In en, this message translates to:
  /// **'PIN access disabled. Use seed recovery to regain access.'**
  String get panicMessage;

  /// No description provided for @seedWarning.
  ///
  /// In en, this message translates to:
  /// **'IMPORTANT: Write down your seed phrase and store it securely. This is the only way to recover your profile.'**
  String get seedWarning;

  /// No description provided for @verifySeedWords.
  ///
  /// In en, this message translates to:
  /// **'Verify your seed phrase by selecting the 3 requested words:'**
  String get verifySeedWords;

  /// No description provided for @confirmPin.
  ///
  /// In en, this message translates to:
  /// **'Confirm PIN'**
  String get confirmPin;

  /// No description provided for @pinMismatch.
  ///
  /// In en, this message translates to:
  /// **'PINs do not match'**
  String get pinMismatch;

  /// No description provided for @invalidPinLength.
  ///
  /// In en, this message translates to:
  /// **'PIN must be 8 digits'**
  String get invalidPinLength;

  /// No description provided for @registrationComplete.
  ///
  /// In en, this message translates to:
  /// **'Registration complete'**
  String get registrationComplete;

  /// No description provided for @language.
  ///
  /// In en, this message translates to:
  /// **'Language'**
  String get language;

  /// No description provided for @english.
  ///
  /// In en, this message translates to:
  /// **'English'**
  String get english;

  /// No description provided for @systemLanguage.
  ///
  /// In en, this message translates to:
  /// **'System Language'**
  String get systemLanguage;

  /// No description provided for @loading.
  ///
  /// In en, this message translates to:
  /// **'Loading...'**
  String get loading;

  /// No description provided for @error.
  ///
  /// In en, this message translates to:
  /// **'Error'**
  String get error;

  /// No description provided for @success.
  ///
  /// In en, this message translates to:
  /// **'Success'**
  String get success;

  /// No description provided for @cancel.
  ///
  /// In en, this message translates to:
  /// **'Cancel'**
  String get cancel;

  /// No description provided for @continueAction.
  ///
  /// In en, this message translates to:
  /// **'Continue'**
  String get continueAction;

  /// No description provided for @back.
  ///
  /// In en, this message translates to:
  /// **'Back'**
  String get back;

  /// No description provided for @next.
  ///
  /// In en, this message translates to:
  /// **'Next'**
  String get next;

  /// No description provided for @russianDetected.
  ///
  /// In en, this message translates to:
  /// **'Russian detected'**
  String get russianDetected;

  /// No description provided for @englishDetected.
  ///
  /// In en, this message translates to:
  /// **'English detected'**
  String get englishDetected;
}

class _AppLocalizationsDelegate extends LocalizationsDelegate<AppLocalizations> {
  const _AppLocalizationsDelegate();

  @override
  Future<AppLocalizations> load(Locale locale) {
    return SynchronousFuture<AppLocalizations>(lookupAppLocalizations(locale));
  }

  @override
  bool isSupported(Locale locale) => <String>['en', 'ru'].contains(locale.languageCode);

  @override
  bool shouldReload(_AppLocalizationsDelegate old) => false;
}

AppLocalizations lookupAppLocalizations(Locale locale) {


  // Lookup logic when only language code is specified.
  switch (locale.languageCode) {
    case 'en': return AppLocalizationsEn();
    case 'ru': return AppLocalizationsRu();
  }

  throw FlutterError(
    'AppLocalizations.delegate failed to load unsupported locale "$locale". This is likely '
    'an issue with the localizations generation tool. Please file an issue '
    'on GitHub with a reproducible sample app and the gen-l10n configuration '
    'that was used.'
  );
}
