// ignore: unused_import
import 'package:intl/intl.dart' as intl;
import 'app_localizations.dart';

// ignore_for_file: type=lint

/// The translations for Russian (`ru`).
class AppLocalizationsRu extends AppLocalizations {
  AppLocalizationsRu([String locale = 'ru']) : super(locale);

  @override
  String get appTitle => 'Royal Control';

  @override
  String get bootMessage => 'Загрузка Royal Control';

  @override
  String get networkBridgeSetup => 'Настройка сетевого моста';

  @override
  String get testingNetworkEnvironment => 'Проверка свойств сетевого окружения...';

  @override
  String get connectingProtocols => 'Подключение настроенных протоколов...';

  @override
  String testingProtocol(Object protocol) {
    return 'Тестирование $protocol...';
  }

  @override
  String protocolSuccess(Object protocol) {
    return '$protocol - OK';
  }

  @override
  String protocolFailed(Object protocol) {
    return '$protocol - Ошибка';
  }

  @override
  String get selectingOptimalSequence => 'Выбор оптимальной последовательности протоколов...';

  @override
  String get preferredSequence => 'Предпочтительная последовательность: v2ray → tor';

  @override
  String get testingSimplexNode => 'Проверка доступности Simplex ноды...';

  @override
  String get simplexNodeAvailable => 'Simplex нода доступна';

  @override
  String get simplexNodeUnavailable => 'Simplex нода недоступна';

  @override
  String get bridgeConfiguration => 'Конфигурация моста';

  @override
  String get configureProtocols => 'Настроить протоколы';

  @override
  String get loadConfigFromFile => 'Загрузить конфиг из файла';

  @override
  String get saveConfiguration => 'Сохранить конфигурацию';

  @override
  String get startBridge => 'Запустить мост';

  @override
  String get bridgeStatus => 'Статус моста';

  @override
  String get protocolStatus => 'Статус протоколов';

  @override
  String get serviceStatus => 'Статус сервисов';

  @override
  String get onboardingTitle => 'Добро пожаловать в Royal Control';

  @override
  String get createNewProfile => 'Создать новый профиль';

  @override
  String get recoverFromSeed => 'Восстановить из сид-фразы';

  @override
  String get selectExistingProfile => 'Выбрать существующий профиль';

  @override
  String get profilesList => 'Доступные профили';

  @override
  String get enterPin => 'Введите PIN';

  @override
  String get pinHint => '8-значный PIN';

  @override
  String get showPin => 'Показать PIN';

  @override
  String get hidePin => 'Скрыть PIN';

  @override
  String get unlock => 'Разблокировать';

  @override
  String get welcomeBack => 'С возвращением';

  @override
  String get dashboard => 'Панель управления';

  @override
  String get settings => 'Настройки';

  @override
  String get panicMode => 'Режим паники активирован';

  @override
  String get panicMessage => 'Доступ по PIN отключен. Используйте восстановление по сид-фразе.';

  @override
  String get seedWarning => 'ВАЖНО: Запишите свою сид-фразу и храните её в безопасном месте. Это единственный способ восстановить профиль.';

  @override
  String get verifySeedWords => 'Подтвердите сид-фразу, выбрав 3 запрошенных слова:';

  @override
  String get confirmPin => 'Подтвердить PIN';

  @override
  String get pinMismatch => 'PIN-коды не совпадают';

  @override
  String get invalidPinLength => 'PIN должен состоять из 8 цифр';

  @override
  String get registrationComplete => 'Регистрация завершена';

  @override
  String get language => 'Язык';

  @override
  String get english => 'Английский';

  @override
  String get systemLanguage => 'Системный язык';

  @override
  String get loading => 'Загрузка...';

  @override
  String get error => 'Ошибка';

  @override
  String get success => 'Успех';

  @override
  String get cancel => 'Отмена';

  @override
  String get continueAction => 'Продолжить';

  @override
  String get back => 'Назад';

  @override
  String get next => 'Далее';

  @override
  String get russianDetected => 'Русский язык обнаружен';

  @override
  String get englishDetected => 'Английский язык обнаружен';
}
