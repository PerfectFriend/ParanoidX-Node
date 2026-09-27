/// Service for managing sovereign identities.
library models.src.identity_service;

import 'dart:convert';
import 'dart:io';
import 'dart:math' show Random;
import 'dart:typed_data';

import 'package:bip39/bip39.dart' as bip39;
import 'package:bip32/bip32.dart' as bip32;
import 'package:crypto/crypto.dart';
import 'package:convert/convert.dart' show hex;
import 'package:ed25519_hd_key/ed25519_hd_key.dart' as ed25519_hd_key;
import 'package:pinenacl/ed25519.dart' as pinenacl_ed25519;
import 'package:shared_preferences/shared_preferences.dart';
import 'package:pointycastle/pointycastle.dart';
import 'package:pointycastle/api.dart' as pc_api;

import 'identity.dart';

/// Service for managing sovereign identities.
class SecureIdentityService {
  static SecureIdentityService? _instance;
  SharedPreferences? _prefs;
  String _deviceKey = '';
  bool _initialized = false;

  // DEBUG: Enable detailed logging
  static const bool _debugLogging = true;

  SecureIdentityService._();

  /// Get singleton instance (initializes on first call)
  static Future<SecureIdentityService> get instance async {
    _instance ??= SecureIdentityService._();
    if (!_instance!._initialized) {
      await _instance!._init();
    }
    return _instance!;
  }

  /// Debug logging helper
  void _log(String message) {
    if (_debugLogging) {
      final timestamp = DateTime.now().toIso8601String();
      print('[SecureIdentityService][$timestamp] $message');
    }
  }

  /// Initialize the service (derive device key, open preferences)
  Future<void> _init() async {
    if (_initialized) return;

    _prefs = await SharedPreferences.getInstance();
    _deviceKey = await _deriveDeviceKey();
    _initialized = true;
    _log('INIT: Service initialized. Device key length: ${_deviceKey.length}');
    _log('INIT: Device key prefix: ${_deviceKey.substring(0, 16)}...');
  }

  /// Derive a device-bound encryption key from hardware identifiers
  Future<String> _deriveDeviceKey() async {
    final buffer = StringBuffer('simplex-identity-salt-v1');

    // Machine ID (Linux)
    try {
      final machineId = await File('/etc/machine-id').readAsString();
      buffer.write(machineId.trim());
      _log('DEVICE_KEY: Found machine-id');
    } catch (_) {
      _log('DEVICE_KEY: No machine-id found');
    }

    // Hostname
    try {
      final hostname = await Process.run('hostname', []);
      buffer.write(hostname.stdout.toString().trim());
      _log('DEVICE_KEY: Found hostname');
    } catch (_) {
      _log('DEVICE_KEY: No hostname');
    }

    // User ID
    try {
      final uid = await Process.run('id', ['-u']);
      buffer.write(uid.stdout.toString().trim());
      _log('DEVICE_KEY: Found UID');
    } catch (_) {
      _log('DEVICE_KEY: No UID');
    }

    // If all fails, use a constant (less secure but functional)
    if (buffer.length < 50) {
      buffer.write('fallback-device-id');
      _log('DEVICE_KEY: Using fallback');
    }

    final hash = sha256.convert(utf8.encode(buffer.toString()));
    _log('DEVICE_KEY: Final key length: ${hash.toString().length}');
    return hash.toString();
  }

  /// Generate a new BIP39 identity with 24-word mnemonic
  Future<Identity> generateIdentity({
    MnemonicWordCount wordCount = MnemonicWordCount.words24,
    String? label,
    String passphrase = '',
  }) async {
    await _init();

    // Generate entropy (32 bytes for 24 words = 256 bits)
    final entropyBytes = _generateEntropy(wordCount.entropyBits ~/ 8);
    final mnemonic = bip39.entropyToMnemonic(hex.encode(entropyBytes));

    // Derive seed from mnemonic + passphrase
    final seed = bip39.mnemonicToSeed(mnemonic, passphrase: passphrase);

    // Derive keys
    final keys = await _deriveKeys(seed, mnemonic, passphrase);

    // Encrypt mnemonic for storage
    final encryptedMnemonic = await _encryptMnemonic(mnemonic);

    // Generate identity ID from public key
    final id = _generateIdentityId(keys.identityKey.publicKey);

    // Create identity object
    final identity = Identity(
      id: id,
      encryptedMnemonic: encryptedMnemonic,
      derivationPath: DerivationPaths.identity,
      ed25519PubKey: keys.identityKey.publicKeyHex,
      x25519PubKey: keys.encryptionKey.publicKeyHex,
      createdAt: DateTime.now().millisecondsSinceEpoch,
      label: label,
      wordCount: wordCount,
    );

    // Store identity
    await _storeIdentity(identity);

    return identity;
  }

  /// Import identity from mnemonic phrase
  Future<Identity> importIdentity({
    required String mnemonic,
    String passphrase = '',
    String? label,
  }) async {
    await _init();

    // Validate mnemonic
    if (!bip39.validateMnemonic(mnemonic)) {
      throw ArgumentError('Invalid BIP39 mnemonic');
    }

    final wordCount = mnemonic.split(' ').length;
    final mnemonicWordCount = MnemonicWordCount.values.firstWhere(
      (e) => e.value == wordCount,
      orElse: () => MnemonicWordCount.words24,
    );

    // Derive seed
    final seed = bip39.mnemonicToSeed(mnemonic, passphrase: passphrase);

    // Derive keys
    final keys = await _deriveKeys(seed, mnemonic, passphrase);

    // Encrypt mnemonic for storage
    final encryptedMnemonic = await _encryptMnemonic(mnemonic);

    // Generate identity ID
    final id = _generateIdentityId(keys.identityKey.publicKey);

    // Create identity
    final identity = Identity(
      id: id,
      encryptedMnemonic: encryptedMnemonic,
      derivationPath: DerivationPaths.identity,
      ed25519PubKey: keys.identityKey.publicKeyHex,
      x25519PubKey: keys.encryptionKey.publicKeyHex,
      createdAt: DateTime.now().millisecondsSinceEpoch,
      label: label,
      wordCount: mnemonicWordCount,
    );

    // Store
    await _storeIdentity(identity);

    return identity;
  }

  /// Export mnemonic (requires identity verification)
  Future<String?> exportMnemonic({
    required String identityId,
    required String passphrase,
  }) async {
    await _init();

    final identity = await getIdentity(identityId);
    if (identity == null) return null;

    // Decrypt mnemonic
    final mnemonic = await _decryptMnemonic(identity.encryptedMnemonic);
    if (mnemonic == null) return null;

    // Verify with passphrase
    final seed = bip39.mnemonicToSeed(mnemonic, passphrase: passphrase);
    final edKey = await ed25519_hd_key.ED25519_HD_KEY.derivePath(DerivationPaths.identity, seed);
    final pubKeyHex = hex.encode(edKey.key);

    if (pubKeyHex != identity.ed25519PubKey) {
      throw ArgumentError('Invalid passphrase');
    }

    return mnemonic;
  }

  /// Get all identities
  Future<List<Identity>> getAllIdentities() async {
    await _init();
    final keys = _prefs!.getKeys().where((k) => k.startsWith('identity_')).toList();
    final identities = <Identity>[];
    
    for (final key in keys) {
      try {
        final jsonStr = _prefs!.getString(key);
        if (jsonStr != null) {
          final json = jsonDecode(jsonStr) as Map<String, dynamic>;
          identities.add(Identity.fromJson(json));
        }
      } catch (_) {}
    }
    
    return identities;
  }

  /// Get current identity (first available, or first in list)
  Future<Identity?> getCurrentIdentity() async {
    final identities = await getAllIdentities();
    return identities.isNotEmpty ? identities.first : null;
  }

  /// Get identity by ID
  ///
  /// Раньше выше была вторая, сломанная копия этого же метода (copy-paste
  /// из getAllIdentities): она собирала весь список и возвращала его, хотя
  /// сигнатура обещает Identity?, из-за чего сборка падала. Дубликат удалён,
  /// осталась единственная корректная реализация.
  Future<Identity?> getIdentity(String identityId) async {
    await _init();

    final jsonStr = _prefs!.getString('identity_$identityId');
    if (jsonStr == null) return null;

    try {
      final json = jsonDecode(jsonStr) as Map<String, dynamic>;
      return Identity.fromJson(json);
    } catch (_) {
      return null;
    }
  }

  /// Delete identity
  Future<void> deleteIdentity(String identityId) async {
    await _init();
    await _prefs!.remove('identity_$identityId');
  }

  /// Set identity label
  Future<void> setIdentityLabel(String identityId, String label) async {
    await _init();

    final identity = await getIdentity(identityId);
    if (identity == null) throw ArgumentError('Identity not found');

    final updated = Identity(
      id: identity.id,
      encryptedMnemonic: identity.encryptedMnemonic,
      derivationPath: identity.derivationPath,
      ed25519PubKey: identity.ed25519PubKey,
      x25519PubKey: identity.x25519PubKey,
      createdAt: identity.createdAt,
      label: label,
      wordCount: identity.wordCount,
    );

    await _storeIdentity(updated);
  }

  /// Verify identity ownership (check mnemonic matches public key)
  Future<bool> verifyIdentity({
    required String identityId,
    required String mnemonic,
    String passphrase = '',
  }) async {
    await _init();

    final identity = await getIdentity(identityId);
    if (identity == null) return false;

    return await identity.verifyMnemonic(mnemonic, passphrase: passphrase);
  }

  /// Derive signing key (Ed25519) for an identity
  Future<ed25519_hd_key.KeyData> getSigningKey({
    required String identityId,
    required String mnemonic,
    String passphrase = '',
    String derivationPath = DerivationPaths.identity,
  }) async {
    final seed = bip39.mnemonicToSeed(mnemonic, passphrase: passphrase);
    return ed25519_hd_key.ED25519_HD_KEY.derivePath(derivationPath, seed);
  }

  /// Derive encryption key (X25519) for an identity
  Future<bip32.BIP32> getEncryptionKey({
    required String identityId,
    required String mnemonic,
    String passphrase = '',
    String derivationPath = DerivationPaths.encryption,
  }) async {
    // Note: BIP32 doesn't natively support X25519
    // This is a placeholder - use a proper X25519 derivation library
    // For now, derive Ed25519 and convert, or use a different approach
    final seed = bip39.mnemonicToSeed(mnemonic, passphrase: passphrase);
    final edKey = await ed25519_hd_key.ED25519_HD_KEY.derivePath(derivationPath, seed);
    // Convert or derive X25519 from Ed25519 seed
    // This is a simplification - in production use x25519_dart or similar
    throw UnimplementedError('X25519 derivation needs dedicated library');
  }

  /// Sign a message with identity's Ed25519 key
  Future<Uint8List> signMessage({
    required String identityId,
    required String mnemonic,
    required Uint8List message,
    String passphrase = '',
    String derivationPath = DerivationPaths.identity,
  }) async {
    final key = await getSigningKey(
      identityId: identityId,
      mnemonic: mnemonic,
      passphrase: passphrase,
      derivationPath: derivationPath,
    );

    // Ed25519 signing using pinenacl
    final signingKey = pinenacl_ed25519.SigningKey.fromSeed(Uint8List.fromList(key.key));
    final signedMessage = signingKey.sign(message);
    return signedMessage.signature.asTypedList;
  }

  /// Verify a signature
  bool verifySignature({
    required String publicKeyHex,
    required Uint8List message,
    required Uint8List signature,
  }) {
    // Ed25519 verification using pinenacl
    final publicKey = Uint8List.fromList(hex.decode(publicKeyHex));
    final verifyKey = pinenacl_ed25519.VerifyKey(publicKey);
    final sig = pinenacl_ed25519.Signature(Uint8List.fromList(signature));
    try {
      return verifyKey.verify(signature: sig, message: message);
    } catch (_) {
      return false;
    }
  }

  /// Verify PIN against stored hash
  Future<bool> verifyPin(String pin, String pinHash) async {
    await _init();
    // Simple hash comparison - in production use proper password hashing
    final hash = sha256.convert(utf8.encode(pin)).toString();
    return hash == pinHash;
  }

  /// Hash PIN for storage
  Future<String> hashPin(String pin) async {
    await _init();
    return sha256.convert(utf8.encode(pin)).toString();
  }

  /// Encrypt mnemonic with PIN-derived key (ChaCha20-Poly1305)
  Future<String> encryptIdentity({
    required String mnemonic,
    required String pin,
  }) async {
    await _init();
    _log('ENCRYPT_IDENTITY: Starting encryption');
    _log('ENCRYPT_IDENTITY: Mnemonic words: ${mnemonic.split(' ').length}');
    _log('ENCRYPT_IDENTITY: PIN length: ${pin.length}');
    final combinedKey = _deviceKey + pin;
    _log('ENCRYPT_IDENTITY: Device key prefix: ${_deviceKey.substring(0, 16)}...');
    _log('ENCRYPT_IDENTITY: Combined key length: ${combinedKey.length}');
    final hash = sha256.convert(utf8.encode(combinedKey));
    final key = Uint8List.fromList(hash.bytes);
    _log('ENCRYPT_IDENTITY: Derived key (hex): ${hex.encode(key)}');
    final nonce = _generateNonce();
    _log('ENCRYPT_IDENTITY: Generated nonce (hex): ${hex.encode(nonce)}');

    final cipher = pc_api.AEADCipher('ChaCha20-Poly1305');
    final params = pc_api.AEADParameters(
      pc_api.KeyParameter(key),
      128,
      nonce,
      Uint8List(0), // associatedData - empty
    );
    cipher.init(true, params);

    final plaintext = utf8.encode(mnemonic);
    _log('ENCRYPT_IDENTITY: Plaintext length: ${plaintext.length}');
    final outputSize = cipher.getOutputSize(plaintext.length);
    final ciphertext = Uint8List(outputSize);
    final len = cipher.processBytes(plaintext, 0, plaintext.length, ciphertext, 0);
    final finalLen = cipher.doFinal(ciphertext, len);
    _log('ENCRYPT_IDENTITY: processBytes wrote: $len, doFinal wrote: $finalLen, total: ${len + finalLen}');
    _log('ENCRYPT_IDENTITY: Ciphertext bytes (hex): ${hex.encode(ciphertext.sublist(0, len + finalLen))}');

    // Combine nonce + ciphertext + tag for storage
    final combined = Uint8List(nonce.length + len + finalLen);
    combined.setAll(0, nonce);
    combined.setAll(nonce.length, ciphertext.sublist(0, len + finalLen));

    final result = base64.encode(combined);
    _log('ENCRYPT_IDENTITY: Final base64 length: ${result.length}');
    _log('ENCRYPT_IDENTITY: Final base64 prefix: ${result.substring(0, 32)}...');
    return result;
  }

  /// Create identity from mnemonic (for seed recovery)
  Future<Identity> createIdentityFromMnemonic({
    required String mnemonic,
    String passphrase = '',
    String? label,
  }) async {
    await _init();

    // Validate mnemonic
    if (!bip39.validateMnemonic(mnemonic)) {
      throw ArgumentError('Invalid BIP39 mnemonic');
    }

    final wordCount = mnemonic.split(' ').length;
    final mnemonicWordCount = MnemonicWordCount.values.firstWhere(
      (e) => e.value == wordCount,
      orElse: () => MnemonicWordCount.words24,
    );

    // Derive seed
    final seed = bip39.mnemonicToSeed(mnemonic, passphrase: passphrase);

    // Derive keys
    final keys = await _deriveKeys(seed, mnemonic, passphrase);

    // Encrypt mnemonic for storage (device-bound, not PIN-bound)
    final encryptedMnemonic = await _encryptMnemonic(mnemonic);

    // Generate identity ID
    final id = _generateIdentityId(keys.identityKey.publicKey);

    // Create identity
    final identity = Identity(
      id: id,
      encryptedMnemonic: encryptedMnemonic,
      derivationPath: DerivationPaths.identity,
      ed25519PubKey: keys.identityKey.publicKeyHex,
      x25519PubKey: keys.encryptionKey.publicKeyHex,
      createdAt: DateTime.now().millisecondsSinceEpoch,
      label: label,
      wordCount: mnemonicWordCount,
    );

    // Store
    await _storeIdentity(identity);

    return identity;
  }

  /// Decrypt identity using PIN
  Future<Identity> decryptIdentity(String encryptedB64, String pin) async {
    await _init();
    _log('DECRYPT_IDENTITY: Starting decryption');
    _log('DECRYPT_IDENTITY: Encrypted B64 length: ${encryptedB64.length}');
    _log('DECRYPT_IDENTITY: Encrypted B64 prefix: ${encryptedB64.substring(0, 32)}...');
    _log('DECRYPT_IDENTITY: PIN length: ${pin.length}');
    // Use the same device key + PIN for decryption
    final combinedKey = _deviceKey + pin;
    _log('DECRYPT_IDENTITY: Device key prefix: ${_deviceKey.substring(0, 16)}...');
    _log('DECRYPT_IDENTITY: Combined key length: ${combinedKey.length}');
    final hash = sha256.convert(utf8.encode(combinedKey));
    final key = Uint8List.fromList(hash.bytes);
    _log('DECRYPT_IDENTITY: Derived key (hex): ${hex.encode(key)}');

    try {
      final combined = base64.decode(encryptedB64);
      _log('DECRYPT_IDENTITY: Decoded combined length: ${combined.length}');
      if (combined.length < 28) throw ArgumentError('Invalid encrypted data'); // 12 nonce + 16 tag minimum

      final nonce = combined.sublist(0, 12);
      final ciphertextWithTag = combined.sublist(12);
      _log('DECRYPT_IDENTITY: Nonce (hex): ${hex.encode(nonce)}');
      _log('DECRYPT_IDENTITY: Ciphertext+tag length: ${ciphertextWithTag.length}');
      _log('DECRYPT_IDENTITY: Ciphertext+tag bytes (hex): ${hex.encode(ciphertextWithTag)}');

      final cipher = pc_api.AEADCipher('ChaCha20-Poly1305');
      final params = pc_api.AEADParameters(
        pc_api.KeyParameter(key),
        128,
        nonce,
        Uint8List(0), // associatedData - empty
      );
      cipher.init(false, params);

      final plaintext = Uint8List(cipher.getOutputSize(ciphertextWithTag.length));
      final len = cipher.processBytes(ciphertextWithTag, 0, ciphertextWithTag.length, plaintext, 0);
      cipher.doFinal(plaintext, len);

      final mnemonic = utf8.decode(plaintext.sublist(0, len));
      _log('DECRYPT_IDENTITY: Decrypted mnemonic length: ${mnemonic.length}');
      _log('DECRYPT_IDENTITY: Decrypted mnemonic words: ${mnemonic.split(' ').length}');

      // Derive identity from mnemonic
      final seed = bip39.mnemonicToSeed(mnemonic);
      final edKey = await ed25519_hd_key.ED25519_HD_KEY.derivePath(DerivationPaths.identity, seed);
      final encKey = await ed25519_hd_key.ED25519_HD_KEY.derivePath(DerivationPaths.encryption, seed);

      // Get public keys from private keys using pinenacl
      final edPubKey = await pinenacl_ed25519.SigningKey.fromSeed(Uint8List.fromList(edKey.key)).publicKey;
      final encPubKey = await pinenacl_ed25519.SigningKey.fromSeed(Uint8List.fromList(encKey.key)).publicKey;

      final id = _generateIdentityId(edPubKey.asTypedList);

      return Identity(
        id: id,
        encryptedMnemonic: encryptedB64,
        derivationPath: DerivationPaths.identity,
        ed25519PubKey: hex.encode(edPubKey.asTypedList),
        x25519PubKey: hex.encode(encPubKey.asTypedList),
        createdAt: DateTime.now().millisecondsSinceEpoch,
      );
    } catch (e) {
      _log('DECRYPT_IDENTITY: ERROR - $e');
      throw ArgumentError('Failed to decrypt identity: $e');
    }
  }

  // ============ Private helpers ============

  /// Generate cryptographically secure entropy
  Uint8List _generateEntropy(int bytes) {
    final random = Random.secure();
    return Uint8List.fromList(List.generate(bytes, (_) => random.nextInt(256)));
  }

  /// Derive all keys from seed
  Future<DerivedKeys> _deriveKeys(Uint8List seed, String mnemonic, String passphrase) async {
    // Ed25519 identity key (BIP44 path: m/44'/1337'/0'/0/0)
    final edKey = await ed25519_hd_key.ED25519_HD_KEY.derivePath(DerivationPaths.identity, seed);
    final encKey = await ed25519_hd_key.ED25519_HD_KEY.derivePath(DerivationPaths.encryption, seed);

    // Get public keys from private keys using pinenacl
    final edPubKey = await pinenacl_ed25519.SigningKey.fromSeed(Uint8List.fromList(edKey.key)).publicKey;
    final encPubKey = await pinenacl_ed25519.SigningKey.fromSeed(Uint8List.fromList(encKey.key)).publicKey;

    return DerivedKeys(
      seed: seed,
      identityKey: KeyPair(
        privateKey: Uint8List.fromList(edKey.key),
        publicKey: edPubKey.asTypedList,
      ),
      encryptionKey: EncryptionKeyPair(
        privateKey: Uint8List.fromList(encKey.key),
        publicKey: encPubKey.asTypedList,
      ),
      mnemonic: mnemonic,
      passphrase: passphrase,
    );
  }

  /// Encrypt mnemonic with device-bound key (ChaCha20-Poly1305)
  Future<String> _encryptMnemonic(String mnemonic) async {
    final key = _getEncryptionKey();
    final nonce = _generateNonce();

    final cipher = pc_api.AEADCipher('ChaCha20-Poly1305');
    final params = pc_api.AEADParameters(
      pc_api.KeyParameter(key),
      128,
      nonce,
      Uint8List(0), // associatedData - empty
    );
    cipher.init(true, params);

    final plaintext = utf8.encode(mnemonic);
    final outputSize = cipher.getOutputSize(plaintext.length);
    final ciphertext = Uint8List(outputSize);
    final len = cipher.processBytes(plaintext, 0, plaintext.length, ciphertext, 0);
    final finalLen = cipher.doFinal(ciphertext, len);

    // Combine nonce + ciphertext + tag for storage
    final combined = Uint8List(nonce.length + len + finalLen);
    combined.setAll(0, nonce);
    combined.setAll(nonce.length, ciphertext.sublist(0, len + finalLen));

    return base64.encode(combined);
  }

  /// Decrypt mnemonic
  Future<String?> _decryptMnemonic(String encryptedB64) async {
    try {
      final combined = base64.decode(encryptedB64);
      if (combined.length < 28) return null; // 12 nonce + 16 tag minimum

      final nonce = combined.sublist(0, 12);
      final ciphertextWithTag = combined.sublist(12);

      final key = _getEncryptionKey();
      final cipher = pc_api.AEADCipher('ChaCha20-Poly1305');
      final params = pc_api.AEADParameters(
        pc_api.KeyParameter(key),
        128,
        nonce,
        Uint8List(0), // associatedData - empty
      );
      cipher.init(false, params);

      final plaintext = Uint8List(cipher.getOutputSize(ciphertextWithTag.length));
      final len = cipher.processBytes(ciphertextWithTag, 0, ciphertextWithTag.length, plaintext, 0);
      cipher.doFinal(plaintext, len);

      return utf8.decode(plaintext.sublist(0, len));
    } catch (_) {
      return null;
    }
  }

  /// Get encryption key (32 bytes from device key)
  Uint8List _getEncryptionKey() {
    final hash = sha256.convert(utf8.encode(_deviceKey));
    return Uint8List.fromList(hash.bytes);
  }

  /// Generate 12-byte nonce for ChaCha20-Poly1305
  Uint8List _generateNonce() {
    final random = Random.secure();
    return Uint8List.fromList(List.generate(12, (_) => random.nextInt(256)));
  }

  /// Generate identity ID from Ed25519 public key
  String _generateIdentityId(Uint8List publicKey) {
    final hash = sha256.convert(publicKey);
    return hash.toString();
  }

  /// Store identity in SharedPreferences
  Future<void> _storeIdentity(Identity identity) async {
    await _prefs!.setString('identity_${identity.id}', jsonEncode(identity.toJson()));
  }
}

/// Convenience function to get the service instance
Future<SecureIdentityService> getIdentityService() => SecureIdentityService.instance;