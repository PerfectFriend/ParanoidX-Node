import 'dart:async';
import 'dart:io';
import 'package:flutter/material.dart';
import 'package:http/http.dart' as http;
import 'package:models/models.dart' show Identity;
import '../services/secure_prefs.dart';
import '../services/identity_service.dart';

const Map<String, String> _locales = {
  'en': 'English',
  'ru': 'Русский',
  'es': 'Español',
  'zh': '中文',
};

const Map<String, Map<String, String>> _strings = {
  'en': {
    'welcome': 'The Island',
    'tagline': 'Private Sovereign Network',
    'disclaimer': 'Self-sovereign digital commonwealth. Experimental software — no warranty. You alone control your keys and assets.',
    'create_new': 'Create New Identity',
    'restore_identity': 'Restore Identity',
    'set_pin': 'Set 6-digit PIN',
    'enter_pin': 'Enter PIN',
    'pin_warning': 'PIN cannot be reset. If forgotten, identity is lost forever.',
    'confirm_pin': 'Confirm PIN',
    'pin_mismatch': 'PINs do not match',
    'wrong_pin': 'Wrong PIN. Attempts:',
    'locked': 'Locked 30s',
    'unlock': 'Unlock',
    'save': 'Save',
    'language': 'Language',
    'visit': 'stmaria.org',
    'connecting': 'Connecting...',
    'tor': 'Tor',
    'dashboard': 'Node',
    'services': 'Services',
    'retry': 'Retry',
    'no_connection': 'Cannot connect to node services.',
    'seed_login': 'Login with Seed Phrase',
    'seed_phrase': 'Enter 24-word seed phrase',
    'seed_wrong': 'Seed phrase does not match stored mnemonic',
    'seed_back': 'Back to PIN',
    'verify_mnemonic': 'Verify Your Mnemonic',
    'verify_instruction': 'Select the correct words to verify your 24-word mnemonic:',
    'mnemonic_warning': 'Write these 24 words down in order. Store them safely offline. Anyone with these words controls your identity and assets.',
    'next': 'Next',
    'verify': 'Verify',
    'done': 'Done',
    'skip': 'Skip',
    'create': 'Create',
    'import': 'Import',
    'wallet_created': 'Wallet created successfully',
    'backup_warning': 'Back up your mnemonic now! You cannot recover without it.',
    'copied': 'Copied to clipboard',
    'copy': 'Copy',
    'verifying': 'Verifying...',
    'tor_check': 'Checking Tor...',
    'tor_ok': 'Tor connected',
    'tor_fail': 'Tor check failed',
    'node_check': 'Checking node...',
    'node_ok': 'Node connected',
    'node_fail': 'Node check failed',
    'welcome_to_isle': 'Welcome to Isle',
    'your_island_awaits': 'Your private digital island awaits.',
  },
  'ru': {
    'welcome': 'Остров',
    'tagline': 'Частный Суверенный Сетевой Проект',
    'disclaimer': 'Самосуверенное цифровое сообщество. Экспериментальное ПО — без гарантий. Только вы контролируете свои ключи и активы.',
    'create_new': 'Создать новую личность',
    'restore_identity': 'Восстановить личность',
    'set_pin': 'Установить 6-значный PIN',
    'enter_pin': 'Введите PIN',
    'pin_warning': 'PIN нельзя сбросить. Если забудете — личность потеряна навсегда.',
    'confirm_pin': 'Подтвердите PIN',
    'pin_mismatch': 'PIN-коды не совпадают',
    'wrong_pin': 'Неверный PIN. Попыток:',
    'locked': 'Заблокировано на 30с',
    'unlock': 'Разблокировать',
    'save': 'Сохранить',
    'language': 'Язык',
    'visit': 'stmaria.org',
    'connecting': 'Подключение...',
    'tor': 'Tor',
    'dashboard': 'Узел',
    'services': 'Сервисы',
    'retry': 'Повторить',
    'no_connection': 'Не удается подключиться к сервисам узла.',
    'seed_login': 'Вход по сид-фразе',
    'seed_phrase': 'Введите 24 слова сид-фразы',
    'seed_wrong': 'Сид-фраза не совпадает с сохраненной мнемоникой',
    'seed_back': 'Назад к PIN',
    'verify_mnemonic': 'Проверьте вашу мнемонику',
    'verify_instruction': 'Выберите правильные слова для проверки вашей 24-словной мнемоники:',
    'mnemonic_warning': 'Запишите эти 24 слова в порядке. Храните их безопасно офлайн. Кто угадает эти слова — контролирует вашу личность и активы.',
    'next': 'Далее',
    'verify': 'Проверить',
    'done': 'Готово',
    'skip': 'Пропустить',
    'create': 'Создать',
    'import': 'Импорт',
    'wallet_created': 'Кошелек успешно создан',
    'backup_warning': 'Сделайте резервную копию мнемоники сейчас! Без неё восстановление невозможно.',
    'copied': 'Скопировано в буфер обмена',
    'copy': 'Копировать',
    'verifying': 'Проверка...',
    'tor_check': 'Проверка Tor...',
    'tor_ok': 'Tor подключен',
    'tor_fail': 'Ошибка проверки Tor',
    'node_check': 'Проверка узла...',
    'node_ok': 'Узел подключен',
    'node_fail': 'Ошибка проверки узла',
    'welcome_to_isle': 'Добро пожаловать на Остров',
    'your_island_awaits': 'Ваш частный цифровой остров ждёт.',
  },
  'es': {
    'welcome': 'La Isla',
    'tagline': 'Red Privada Soberana',
    'disclaimer': 'Commonwealth digital autosoberano. Software experimental — sin garantía. Solo usted controla sus claves y activos.',
    'create_new': 'Crear Nueva Identidad',
    'restore_identity': 'Restaurar Identidad',
    'set_pin': 'Establecer PIN de 6 dígitos',
    'enter_pin': 'Ingresar PIN',
    'pin_warning': 'El PIN no se puede restablecer. Si lo olvida, la identidad se pierde para siempre.',
    'confirm_pin': 'Confirmar PIN',
    'pin_mismatch': 'Los PIN no coinciden',
    'wrong_pin': 'PIN incorrecto. Intentos:',
    'locked': 'Bloqueado 30s',
    'unlock': 'Desbloquear',
    'save': 'Guardar',
    'language': 'Idioma',
    'visit': 'stmaria.org',
    'connecting': 'Conectando...',
    'tor': 'Tor',
    'dashboard': 'Nodo',
    'services': 'Servicios',
    'retry': 'Reintentar',
    'no_connection': 'No se puede conectar a los servicios del nodo.',
    'seed_login': 'Iniciar con Frase Semilla',
    'seed_phrase': 'Ingrese frase semilla de 24 palabras',
    'seed_wrong': 'La frase semilla no coincide con la mnemónica almacenada',
    'seed_back': 'Volver al PIN',
    'verify_mnemonic': 'Verifique su Mnemónico',
    'verify_instruction': 'Seleccione las palabras correctas para verificar su mnemónico de 24 palabras:',
    'mnemonic_warning': 'Escriba estas 24 palabras en orden. Guárdelas offline de forma segura. Cualquiera con estas palabras controla su identidad y activos.',
    'next': 'Siguiente',
    'verify': 'Verificar',
    'done': 'Hecho',
    'skip': 'Omitir',
    'create': 'Crear',
    'import': 'Importar',
    'wallet_created': 'Billetera creada exitosamente',
    'backup_warning': '¡Haga respaldo de su mnemónico ahora! No puede recuperar sin él.',
    'copied': 'Copiado al portapapeles',
    'copy': 'Copiar',
    'verifying': 'Verificando...',
    'tor_check': 'Verificando Tor...',
    'tor_ok': 'Tor conectado',
    'tor_fail': 'Fallo verificación Tor',
    'node_check': 'Verificando nodo...',
    'node_ok': 'Nodo conectado',
    'node_fail': 'Fallo verificación nodo',
    'welcome_to_isle': 'Bienvenido a Isle',
    'your_island_awaits': 'Su isla digital privada le espera.',
  },
  'zh': {
    'welcome': '岛屿',
    'tagline': '私有主权网络',
    'disclaimer': '自我主权数字共同体。实验性软件 — 无担保。只有您控制您的密钥和资产。',
    'create_new': '创建新身份',
    'restore_identity': '恢复身份',
    'set_pin': '设置 6 位 PIN 码',
    'enter_pin': '输入 PIN 码',
    'pin_warning': 'PIN 无法重置。遗忘则身份永久丢失。',
    'confirm_pin': '确认 PIN 码',
    'pin_mismatch': '两次 PIN 不一致',
    'wrong_pin': 'PIN 错误。剩余尝试:',
    'locked': '锁定 30秒',
    'unlock': '解锁',
    'save': '保存',
    'language': '语言',
    'visit': 'stmaria.org',
    'connecting': '连接中...',
    'tor': 'Tor',
    'dashboard': '节点',
    'services': '服务',
    'retry': '重试',
    'no_connection': '无法连接到节点服务。',
    'seed_login': '使用助记词登录',
    'seed_phrase': '输入 24 词助记词',
    'seed_wrong': '助记词与存储的助记词不匹配',
    'seed_back': '返回 PIN',
    'verify_mnemonic': '验证您的助记词',
    'verify_instruction': '选择正确的单词以验证您的 24 词助记词：',
    'mnemonic_warning': '按顺序抄写这 24 个词。离线安全保存。持有这些词即控制您的身份和资产。',
    'next': '下一步',
    'verify': '验证',
    'done': '完成',
    'skip': '跳过',
    'create': '创建',
    'import': '导入',
    'wallet_created': '钱包创建成功',
    'backup_warning': '立即备份助记词！没有它无法恢复。',
    'copied': '已复制到剪贴板',
    'copy': '复制',
    'verifying': '验证中...',
    'tor_check': '检查 Tor...',
    'tor_ok': 'Tor 已连接',
    'tor_fail': 'Tor 检查失败',
    'node_check': '检查节点...',
    'node_ok': '节点已连接',
    'node_fail': '节点检查失败',
    'welcome_to_isle': '欢迎来到 Isle',
    'your_island_awaits': '您的私有数字岛屿在等待。',
  },
};

String _t(BuildContext context, String key) {
  final locale = Localizations.localeOf(context).languageCode;
  return _strings[locale]?[key] ?? _strings['en']![key] ?? key;
}

enum WelcomeMode {
  pin,
  createPin,
  mnemonicDisplay,
  verifyMnemonic,
  seedLogin,
  connecting,
  error,
}

class WelcomeScreen extends StatefulWidget {
  final String initialLocale;

  const WelcomeScreen({super.key, this.initialLocale = 'en'});

  @override
  State<WelcomeScreen> createState() => _WelcomeScreenState();
}

class _WelcomeScreenState extends State<WelcomeScreen> with WidgetsBindingObserver {
  late String _locale;
  late WelcomeMode _mode;
  final _pinCtrl = TextEditingController();
  final _confirmCtrl = TextEditingController();
  final _seedCtrl = TextEditingController();
  late http.Client _httpClient;
  bool _loading = false;
  String? _error;

  // BIP39 Identity flow
  Identity? _identity;
  List<String> _mnemonicWords = [];
  List<int> _verifyIndices = [];
  List<String> _verifyOptions = [];
  int _verifyStep = 0;
  bool _verifyComplete = false;
  bool _mnemonicSaved = false;

  // Connection state
  bool _bgTorOk = false;
  bool _bgTorChecking = false;
  bool _bgDashOk = false;
  bool _bgServicesOk = false;
  String? _nodeHealth;
  String? _smpAddr;
  String? _xftpAddr;
  String? _radioAddr;
  String? _jokeText;
  bool _pinSetupDone = false;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _locale = widget.initialLocale;
    _mode = WelcomeMode.pin;
    _httpClient = http.Client();
    _checkExistingIdentity();
    _startBackgroundChecks();
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _pinCtrl.dispose();
    _confirmCtrl.dispose();
    _seedCtrl.dispose();
    _httpClient.close();
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.paused) {
      // App backgrounded
    } else if (state == AppLifecycleState.resumed) {
      _startBackgroundChecks();
    }
  }

  Future<void> _checkExistingIdentity() async {
    try {
      final prefs = await SecurePrefs.instance;
      final hasIdentity = await prefs.containsKey('identity_encrypted');
      final pinSet = await prefs.containsKey('pin_hash');
      if (hasIdentity && pinSet) {
        if (mounted) setState(() => _mode = WelcomeMode.pin);
      } else if (!hasIdentity) {
        if (mounted) setState(() => _mode = WelcomeMode.createPin);
      } else {
        if (mounted) setState(() => _mode = WelcomeMode.createPin);
      }
    } catch (e) {
      if (mounted) setState(() {
        _mode = WelcomeMode.error;
        _error = e.toString();
      });
    }
  }

  Future<void> _startBackgroundChecks() async {
    if (_bgTorChecking) return;
    setState(() => _bgTorChecking = true);

    try {
      // Check Tor
      final torResp = await _httpClient.get(Uri.parse('http://127.0.0.1:9050')).timeout(const Duration(seconds: 5));
      if (mounted) setState(() => _bgTorOk = torResp.statusCode == 200);
    } catch (_) {
      if (mounted) setState(() => _bgTorOk = false);
    }

    try {
      // Check dashboard
      final dashResp = await _httpClient.get(Uri.parse('http://localhost:8080/api/status')).timeout(const Duration(seconds: 5));
      if (mounted) setState(() => _bgDashOk = dashResp.statusCode == 200);
    } catch (_) {
      if (mounted) setState(() => _bgDashOk = false);
    }

    try {
      // Check services
      final svcResp = await _httpClient.get(Uri.parse('http://localhost:8080/api/services')).timeout(const Duration(seconds: 5));
      if (mounted) setState(() => _bgServicesOk = svcResp.statusCode == 200);
    } catch (_) {
      if (mounted) setState(() => _bgServicesOk = false);
    }

    if (mounted) setState(() => _bgTorChecking = false);
  }

  Future<void> _createIdentity() async {
    setState(() => _loading = true);
    try {
      final identityService = SecureIdentityService.instance;
      final identity = await identityService.generateIdentity();
      final prefs = await SecurePrefs.instance;
      await prefs.setIdentity(identity);
      await prefs.setHashed('pin_hash', _pinCtrl.text);
      if (mounted) {
        setState(() {
          _identity = identity;
          _mnemonicWords = identity.encryptedMnemonic.split(' '); // Note: this is encrypted, not plain mnemonic
          _mode = WelcomeMode.mnemonicDisplay;
          _loading = false;
        });
      }
    } catch (e) {
      if (mounted) setState(() {
        _error = e.toString();
        _mode = WelcomeMode.error;
        _loading = false;
      });
    }
  }

  Future<void> _restoreIdentity() async {
    setState(() => _loading = true);
    try {
      final words = _seedCtrl.text.trim().split(RegExp(r'\s+'));
      if (words.length != 24) throw Exception('Must be exactly 24 words');
      final identityService = SecureIdentityService.instance;
      final identity = await identityService.restoreIdentity(words.join(' '));
      final prefs = await SecurePrefs.instance;
      await prefs.setIdentity(identity);
      await prefs.setHashed('pin_hash', _pinCtrl.text);
      if (mounted) {
        setState(() {
          _identity = identity;
          _mode = WelcomeMode.pin;
          _loading = false;
        });
      }
    } catch (e) {
      if (mounted) setState(() {
        _error = e.toString();
        _mode = WelcomeMode.error;
        _loading = false;
      });
    }
  }

  void _verifyMnemonicStep() {
    if (_verifyStep >= 3) {
      setState(() {
        _verifyComplete = true;
        _mode = WelcomeMode.pin;
      });
      return;
    }

    final correctWord = _mnemonicWords[_verifyIndices[_verifyStep]];
    final options = [correctWord];
    while (options.length < 4) {
      final randomWord = _mnemonicWords[(DateTime.now().millisecondsSinceEpoch + options.length) % _mnemonicWords.length];
      if (!options.contains(randomWord)) options.add(randomWord);
    }
    options.shuffle();

    setState(() {
      _verifyOptions = options;
    });
  }

  void _startMnemonicVerification() {
    _verifyIndices = [5, 12, 19]; // Fixed positions for verification
    _verifyStep = 0;
    _verifyMnemonicStep();
    setState(() => _mode = WelcomeMode.verifyMnemonic);
  }

  void _onVerifySelection(String word) {
    if (word == _mnemonicWords[_verifyIndices[_verifyStep]]) {
      _verifyStep++;
      if (_verifyStep >= 3) {
        _verifyComplete = true;
        setState(() => _mode = WelcomeMode.pin);
      } else {
        _verifyMnemonicStep();
      }
    } else {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(_t(context, 'wrong_pin')), backgroundColor: Colors.red),
      );
    }
  }

  Future<void> _unlockWithPin() async {
    if (_pinCtrl.text.length != 6) return;
    setState(() => _loading = true);
    try {
      final prefs = await SecurePrefs.instance;
      final valid = await prefs.verify('pin_hash', _pinCtrl.text);
      if (valid) {
        final identity = await prefs.getIdentity();
        if (mounted) {
          setState(() {
            _identity = identity;
            _mode = WelcomeMode.connecting;
            _loading = false;
          });
          _navigateToMain();
        }
      } else {
        if (mounted) {
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(content: Text('${_t(context, 'wrong_pin')}'), backgroundColor: Colors.red),
          );
        }
      }
    } catch (e) {
      if (mounted) setState(() {
        _error = e.toString();
        _mode = WelcomeMode.error;
        _loading = false;
      });
    }
  }

  void _navigateToMain() {
    if (_identity != null) {
      Navigator.of(context).pushReplacementNamed('/main', arguments: _identity);
    }
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return MaterialApp(
      locale: Locale(_locale),
      theme: theme,
      home: Scaffold(
        body: SafeArea(
          child: _buildBody(theme),
        ),
      ),
    );
  }

  Widget _buildBody(ThemeData theme) {
    switch (_mode) {
      case WelcomeMode.pin:
        return _buildPinScreen(theme);
      case WelcomeMode.createPin:
        return _buildCreatePinScreen(theme);
      case WelcomeMode.mnemonicDisplay:
        return _buildMnemonicDisplay(theme);
      case WelcomeMode.verifyMnemonic:
        return _buildMnemonicVerify(theme);
      case WelcomeMode.seedLogin:
        return _buildSeedLogin(theme);
      case WelcomeMode.connecting:
        return _buildConnecting(theme);
      case WelcomeMode.error:
        return _buildError(theme);
    }
  }

  Widget _buildPinScreen(ThemeData theme) {
    return Center(
      child: SingleChildScrollView(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Image.asset('assets/images/coat-of-arms.jpg',
                width: 96, height: 96, fit: BoxFit.contain),
            const SizedBox(height: 16),
            Text(_t(context, 'welcome'), style: theme.textTheme.headlineMedium),
            Text(_t(context, 'tagline'), style: theme.textTheme.titleMedium?.copyWith(color: theme.colorScheme.primary)),
            const SizedBox(height: 8),
            Text(_t(context, 'disclaimer'), style: theme.textTheme.bodySmall, textAlign: TextAlign.center),
            const SizedBox(height: 32),
            Text(_t(context, 'enter_pin'), style: theme.textTheme.titleLarge),
            const SizedBox(height: 16),
            TextField(
              controller: _pinCtrl,
              keyboardType: TextInputType.number,
              obscureText: true,
              maxLength: 6,
              textAlign: TextAlign.center,
              style: const TextStyle(fontSize: 32, letterSpacing: 8),
              decoration: const InputDecoration(
                counterText: '',
                border: OutlineInputBorder(),
              ),
              onSubmitted: (_) => _unlockWithPin(),
            ),
            const SizedBox(height: 16),
            FilledButton.icon(
              onPressed: _loading ? null : _unlockWithPin,
              icon: _loading ? const SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2)) : const Icon(Icons.lock_open),
              label: Text(_t(context, 'unlock')),
              style: FilledButton.styleFrom(minimumSize: const Size(double.infinity, 48)),
            ),
            const SizedBox(height: 16),
            TextButton(
              onPressed: () => setState(() => _mode = WelcomeMode.seedLogin),
              child: Text(_t(context, 'seed_login')),
            ),
            const SizedBox(height: 8),
            TextButton(
              onPressed: () => setState(() => _locale = _locale == 'en' ? 'ru' : 'en'),
              child: Text('${_t(context, 'language')}: ${_locales[_locale] ?? _locale}'),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildCreatePinScreen(ThemeData theme) {
    return Center(
      child: SingleChildScrollView(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Image.asset('assets/images/coat-of-arms.jpg',
                width: 96, height: 96, fit: BoxFit.contain),
            const SizedBox(height: 16),
            Text(_t(context, 'welcome'), style: theme.textTheme.headlineMedium),
            Text(_t(context, 'tagline'), style: theme.textTheme.titleMedium?.copyWith(color: theme.colorScheme.primary)),
            const SizedBox(height: 8),
            Text(_t(context, 'disclaimer'), style: theme.textTheme.bodySmall, textAlign: TextAlign.center),
            const SizedBox(height: 32),
            Card(
              child: Padding(
                padding: const EdgeInsets.all(16),
                child: Column(
                  children: [
                    Text(_t(context, 'set_pin'), style: theme.textTheme.titleLarge),
                    const SizedBox(height: 8),
                    Text(_t(context, 'pin_warning'), style: theme.textTheme.bodySmall?.copyWith(color: Colors.orange), textAlign: TextAlign.center),
                    const SizedBox(height: 16),
                    TextField(
                      controller: _pinCtrl,
                      keyboardType: TextInputType.number,
                      obscureText: true,
                      maxLength: 6,
                      textAlign: TextAlign.center,
                      style: const TextStyle(fontSize: 32, letterSpacing: 8),
                      decoration: const InputDecoration(counterText: '', border: OutlineInputBorder()),
                    ),
                    const SizedBox(height: 16),
                    TextField(
                      controller: _confirmCtrl,
                      keyboardType: TextInputType.number,
                      obscureText: true,
                      maxLength: 6,
                      textAlign: TextAlign.center,
                      style: const TextStyle(fontSize: 32, letterSpacing: 8),
                      decoration: InputDecoration(
                        counterText: '',
                        border: const OutlineInputBorder(),
                        labelText: _t(context, 'confirm_pin'),
                      ),
                    ),
                    const SizedBox(height: 24),
                    FilledButton(
                      onPressed: _loading ? null : () {
                        if (_pinCtrl.text.length != 6 || _confirmCtrl.text.length != 6) return;
                        if (_pinCtrl.text != _confirmCtrl.text) {
                          ScaffoldMessenger.of(context).showSnackBar(
                            SnackBar(content: Text(_t(context, 'pin_mismatch')), backgroundColor: Colors.red),
                          );
                          return;
                        }
                        _createIdentity();
                      },
                      child: _loading ? const CircularProgressIndicator() : Text(_t(context, 'create')),
                      style: FilledButton.styleFrom(minimumSize: const Size(double.infinity, 48)),
                    ),
                  ],
                ),
              ),
            ),
            const SizedBox(height: 16),
            TextButton(
              onPressed: () => setState(() => _mode = WelcomeMode.seedLogin),
              child: Text(_t(context, 'restore_identity')),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildMnemonicDisplay(ThemeData theme) {
    return Center(
      child: SingleChildScrollView(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(Icons.warning_amber, size: 64, color: Colors.orange),
            const SizedBox(height: 16),
            Text(_t(context, 'wallet_created'), style: theme.textTheme.headlineSmall?.copyWith(color: Colors.green)),
            const SizedBox(height: 16),
            Text(_t(context, 'backup_warning'), style: theme.textTheme.bodyMedium, textAlign: TextAlign.center),
            const SizedBox(height: 24),
            Card(
              child: Padding(
                padding: const EdgeInsets.all(16),
                child: Wrap(
                  spacing: 8,
                  runSpacing: 8,
                  children: _mnemonicWords.asMap().entries.map((e) => Chip(
                    label: Text('${e.key + 1}. ${e.value}'),
                    labelStyle: const TextStyle(fontFamily: 'monospace', fontSize: 12),
                    backgroundColor: theme.colorScheme.surfaceContainerHighest,
                  )).toList(),
                ),
              ),
            ),
            const SizedBox(height: 16),
            Row(
              children: [
                Expanded(
                  child: OutlinedButton.icon(
                    onPressed: () {
                      final text = _mnemonicWords.asMap().entries.map((e) => '${e.key + 1}. ${e.value}').join(' ');
                      ScaffoldMessenger.of(context).showSnackBar(
                        SnackBar(content: Text(_t(context, 'copied'))),
                      );
                    },
                    icon: const Icon(Icons.copy),
                    label: Text(_t(context, 'copy')),
                  ),
                ),
                const SizedBox(width: 16),
                Expanded(
                  child: FilledButton.icon(
                    onPressed: _startMnemonicVerification,
                    icon: const Icon(Icons.check),
                    label: Text(_t(context, 'verify')),
                    style: FilledButton.styleFrom(minimumSize: const Size(double.infinity, 48)),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 16),
            TextButton(
              onPressed: () => setState(() => _mode = WelcomeMode.pin),
              child: Text(_t(context, 'skip')),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildMnemonicVerify(ThemeData theme) {
    final correctWord = _mnemonicWords[_verifyIndices[_verifyStep]];
    return Center(
      child: SingleChildScrollView(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(Icons.verified, size: 64, color: theme.colorScheme.primary),
            const SizedBox(height: 16),
            Text(_t(context, 'verify_mnemonic'), style: theme.textTheme.headlineSmall),
            const SizedBox(height: 8),
            Text(_t(context, 'verify_instruction'), style: theme.textTheme.bodyMedium, textAlign: TextAlign.center),
            const SizedBox(height: 24),
            Text('Word ${_verifyIndices[_verifyStep] + 1} of 24', style: theme.textTheme.titleMedium),
            const SizedBox(height: 16),
            Wrap(
              spacing: 12,
              runSpacing: 12,
              children: _verifyOptions.map((w) => ElevatedButton(
                onPressed: () => _onVerifySelection(w),
                style: ElevatedButton.styleFrom(
                  minimumSize: const Size(100, 48),
                  backgroundColor: w == correctWord ? Colors.green : theme.colorScheme.surfaceContainerHighest,
                ),
                child: Text(w, style: const TextStyle(fontFamily: 'monospace')),
              )).toList(),
            ),
            const SizedBox(height: 16),
            LinearProgressIndicator(value: (_verifyStep + 1) / 3),
            const SizedBox(height: 8),
            Text('Step ${_verifyStep + 1} of 3', style: theme.textTheme.bodySmall),
          ],
        ),
      ),
    );
  }

  Widget _buildSeedLogin(ThemeData theme) {
    return Center(
      child: SingleChildScrollView(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(Icons.restore, size: 64, color: theme.colorScheme.primary),
            const SizedBox(height: 16),
            Text(_t(context, 'restore_identity'), style: theme.textTheme.headlineSmall),
            const SizedBox(height: 8),
            Text(_t(context, 'seed_phrase'), style: theme.textTheme.bodyMedium),
            const SizedBox(height: 24),
            TextField(
              controller: _seedCtrl,
              maxLines: 4,
              minLines: 2,
              decoration: InputDecoration(
                hintText: _t(context, 'seed_phrase'),
                border: const OutlineInputBorder(),
              ),
            ),
            const SizedBox(height: 16),
            FilledButton(
              onPressed: _loading ? null : _restoreIdentity,
              child: _loading ? const CircularProgressIndicator() : Text(_t(context, 'import')),
              style: FilledButton.styleFrom(minimumSize: const Size(double.infinity, 48)),
            ),
            const SizedBox(height: 16),
            TextButton(
              onPressed: () => setState(() => _mode = WelcomeMode.pin),
              child: Text(_t(context, 'seed_back')),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildConnecting(ThemeData theme) {
    return Center(
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          const CircularProgressIndicator(),
          const SizedBox(height: 24),
          Text(_t(context, 'connecting'), style: theme.textTheme.headlineSmall),
          const SizedBox(height: 16),
          Text(_t(context, 'your_island_awaits'), style: theme.textTheme.bodyMedium),
        ],
      ),
    );
  }

  Widget _buildError(ThemeData theme) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(Icons.error_outline, size: 64, color: Colors.red),
            const SizedBox(height: 16),
            Text('Error', style: theme.textTheme.headlineSmall?.copyWith(color: Colors.red)),
            const SizedBox(height: 8),
            Text(_error ?? 'Unknown error', style: theme.textTheme.bodyMedium, textAlign: TextAlign.center),
            const SizedBox(height: 24),
            FilledButton(onPressed: _checkExistingIdentity, child: Text(_t(context, 'retry'))),
          ],
        ),
      ),
    );
  }
}