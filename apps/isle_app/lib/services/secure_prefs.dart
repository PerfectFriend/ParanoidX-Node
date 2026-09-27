import 'dart:convert';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:crypto/crypto.dart';

import 'package:models/models.dart';

/// SecurePrefs provides a simple interface for secure storage,
/// now backed by the shared SecureIdentityService.
/// 
/// For PIN storage, it uses hashed verification.
/// For mnemonic/key storage, it delegates to SecureIdentityService.
class SecurePrefs {
  static SecurePrefs? _instance;
  bool _ready = false;

  SecurePrefs._();

  static Future<SecurePrefs> get instance async {
    if (_instance != null) return _instance!;
    _instance = SecurePrefs._();
    await _instance!._init();
    return _instance!;
  }

  Future<void> _init() async {
    // Ensure identity service is initialized
    await SecureIdentityService.instance;
    _ready = true;
  }

  /// Store a hashed value (one-way, for PIN verification)
  Future<void> setHashed(String key, String value) async {
    if (!_ready) await _init();
    final prefs = await SharedPreferences.getInstance();
    final hash = sha256.convert(utf8.encode(value)).toString();
    await prefs.setString('_sec_$key', hash);
  }

  /// Verify a value against stored hash
  Future<bool> verify(String key, String value) async {
    if (!_ready) await _init();
    final prefs = await SharedPreferences.getInstance();
    final stored = prefs.getString('_sec_$key');
    if (stored == null) return false;
    return stored == sha256.convert(utf8.encode(value)).toString();
  }

  /// Check if a key exists in secure storage
  Future<bool> containsKey(String key) async {
    final prefs = await SharedPreferences.getInstance();
    return prefs.containsKey('_sec_$key');
  }

  /// Remove a key
  Future<void> remove(String key) async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.remove('_sec_$key');
  }

  /// Store a string value (plain, for non-sensitive data)
  Future<void> setString(String key, String value) async {
    if (!_ready) await _init();
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString('_sec_str_$key', value);
  }

  /// Read a string value
  Future<String?> getString(String key) async {
    if (!_ready) await _init();
    final prefs = await SharedPreferences.getInstance();
    return prefs.getString('_sec_str_$key');
  }

  /// Store a boolean value
  Future<void> setBool(String key, bool value) async {
    if (!_ready) await _init();
    final prefs = await SharedPreferences.getInstance();
    await prefs.setBool('_sec_bool_$key', value);
  }

  /// Read a boolean value
  Future<bool?> getBool(String key) async {
    if (!_ready) await _init();
    final prefs = await SharedPreferences.getInstance();
    return prefs.getBool('_sec_bool_$key');
  }

  /// Clear all secure entries
  Future<void> clear() async {
    final prefs = await SharedPreferences.getInstance();
    final keys = prefs.getKeys().where((k) => k.startsWith('_sec_'));
    for (final k in keys) {
      await prefs.remove(k);
    }
  }

  /// Store identity mnemonic (delegates to SecureIdentityService)
  Future<void> storeMnemonic({
    required String mnemonic,
    required String passphrase,
    String? label,
  }) async {
    await SecureIdentityService.instance;
    final service = await SecureIdentityService.instance;
    await service.importIdentity(
      mnemonic: mnemonic,
      passphrase: passphrase,
      label: label,
    );
  }

  /// Get current identity
  Future<Identity?> getCurrentIdentity() async {
    await SecureIdentityService.instance;
    final service = await SecureIdentityService.instance;
    return service.getCurrentIdentity();
  }

  /// Get identity (alias for getCurrentIdentity)
  Future<Identity?> getIdentity() async {
    return getCurrentIdentity();
  }

  /// Get all identities
  Future<List<Identity>> getAllIdentities() async {
    await SecureIdentityService.instance;
    final service = await SecureIdentityService.instance;
    return service.getAllIdentities();
  }

  /// Set identity (stores in shared prefs as JSON)
  Future<void> setIdentity(Identity identity) async {
    if (!_ready) await _init();
    final prefs = await SharedPreferences.getInstance();
    final json = jsonEncode({
      'id': identity.id,
      'ed25519PubKey': identity.ed25519PubKey,
      'x25519PubKey': identity.x25519PubKey,
      'createdAt': identity.createdAt,
      'label': identity.label,
    });
    await prefs.setString('_sec_identity', json);
  }

  /// Verify mnemonic against stored identity
  Future<bool> verifyMnemonic(String identityId, String mnemonic, {String passphrase = ''}) async {
    await SecureIdentityService.instance;
    final service = await SecureIdentityService.instance;
    return service.verifyIdentity(
      identityId: identityId,
      mnemonic: mnemonic,
      passphrase: passphrase,
    );
  }

  /// Export mnemonic (for backup)
  Future<String?> exportMnemonic(String identityId, String passphrase) async {
    await SecureIdentityService.instance;
    final service = await SecureIdentityService.instance;
    return service.exportMnemonic(identityId: identityId, passphrase: passphrase);
  }

  /// Delete identity
  Future<void> deleteIdentity(String identityId) async {
    await SecureIdentityService.instance;
    final service = await SecureIdentityService.instance;
    return service.deleteIdentity(identityId);
  }

  /// Set identity label
  Future<void> setIdentityLabel(String identityId, String label) async {
    await SecureIdentityService.instance;
    final service = await SecureIdentityService.instance;
    return service.setIdentityLabel(identityId, label);
  }
}