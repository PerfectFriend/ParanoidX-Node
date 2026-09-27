import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';
import 'package:crypto/crypto.dart';
import 'package:encrypt/encrypt.dart' as encrypt;
import 'package:bip39/bip39.dart' as bip39;
import 'package:ed25519_hd_key/ed25519_hd_key.dart' show ED25519_HD_KEY, KeyData;
import 'package:convert/convert.dart' show hex;
import 'package:models/models.dart';

/// SecureIdentityService handles BIP39 identity generation and secure storage.
class SecureIdentityService {
  SecureIdentityService._();
  static final SecureIdentityService instance = SecureIdentityService._();

  /// Generate a new BIP39 identity with 24-word mnemonic
  Future<Identity> generateIdentity({
    String passphrase = '',
    String? label,
    MnemonicWordCount wordCount = MnemonicWordCount.words24,
  }) async {
    // Generate mnemonic
    // Раньше: bip39.Mnemonic.generate(wordCount.value) — класса Mnemonic
    // в пакете bip39 нет вообще, а generateMnemonic ждёт биты энтропии
    // (128/256), а не число слов. Связь: words*11 - 4 биты.
    // Ошибка была: "Undefined name 'Mnemonic'".
    final mnemonicStr = bip39.generateMnemonic(strength: wordCount.entropyBits);

    // Derive seed from mnemonic
    final seed = bip39.mnemonicToSeed(mnemonicStr, passphrase: passphrase);

    // Derive Ed25519 identity key (m/44'/1337'/0'/0'/0')
    final masterKey = await ED25519_HD_KEY.getMasterKeyFromSeed(seed);
    final derived = await ED25519_HD_KEY.derivePath("m/44'/1337'/0'/0'/0'", seed);
    final ed25519PubKey = hex.encode(derived.key);

    // Derive X25519 encryption key (m/44'/1337'/0'/1'/0')
    final encKey = await ED25519_HD_KEY.getMasterKeyFromSeed(seed);
    final encDerived = await ED25519_HD_KEY.derivePath("m/44'/1337'/0'/1'/0'", seed);
    final x25519PubKey = hex.encode(encDerived.key);

    // Create identity ID from Ed25519 pubkey
    final id = sha256.convert(hex.decode(ed25519PubKey)).toString();

    // Encrypt mnemonic for storage
    final deviceKey = await _getDeviceKey();
    final encrypter = encrypt.Encrypter(encrypt.AES(deviceKey));
    final encryptedMnemonic = encrypter.encrypt(mnemonicStr, iv: encrypt.IV.fromSecureRandom(16)).base64;

    return Identity(
      id: id,
      encryptedMnemonic: encryptedMnemonic,
      derivationPath: "m/44'/1337'/0'/0'/0'",
      ed25519PubKey: ed25519PubKey,
      x25519PubKey: x25519PubKey,
      createdAt: DateTime.now().millisecondsSinceEpoch,
      label: label,
      wordCount: wordCount,
    );
  }

  /// Decrypt and restore identity from encrypted mnemonic
  Future<Identity> restoreIdentity(String encryptedMnemonic, {String passphrase = ''}) async {
    final deviceKey = await _getDeviceKey();
    final encrypter = encrypt.Encrypter(encrypt.AES(deviceKey));
    final decrypted = encrypter.decrypt64(encryptedMnemonic);

    final mnemonic = decrypted.split(' ');
    final seed = bip39.mnemonicToSeed(mnemonic.join(' '), passphrase: passphrase);

    final masterKey = await ED25519_HD_KEY.getMasterKeyFromSeed(seed);
    final derived = await ED25519_HD_KEY.derivePath("m/44'/1337'/0'/0'/0'", seed);
    final ed25519PubKey = hex.encode(derived.key);

    final encMaster = await ED25519_HD_KEY.getMasterKeyFromSeed(seed);
    final encDerived = await ED25519_HD_KEY.derivePath("m/44'/1337'/0'/1'/0'", seed);
    final x25519PubKey = hex.encode(encDerived.key);

    final id = sha256.convert(hex.decode(ed25519PubKey)).toString();

    return Identity(
      id: id,
      encryptedMnemonic: encryptedMnemonic,
      derivationPath: "m/44'/1337'/0'/0'/0'",
      ed25519PubKey: ed25519PubKey,
      x25519PubKey: x25519PubKey,
      createdAt: DateTime.now().millisecondsSinceEpoch,
    );
  }

  /// Verify mnemonic matches stored identity
  Future<bool> verifyMnemonic(Identity identity, String mnemonic, {String passphrase = ''}) async {
    return identity.verifyMnemonic(mnemonic, passphrase: passphrase);
  }

  /// Get device-derived encryption key
  Future<encrypt.Key> _getDeviceKey() async {
    // In production, this would use platform-specific secure storage
    // For now, derive from device ID + constant
    final deviceId = 'simplex-node-device-12345'; // TODO: Get actual device ID
    final bytes = sha256.convert(utf8.encode(deviceId)).bytes;
    return encrypt.Key(Uint8List.fromList(bytes));
  }

  /// Get current identity (stub — returns null)
  Future<Identity?> getCurrentIdentity() async {
    return null; // In production, load from secure storage
  }

  /// Set identity label (stub)
  Future<void> setIdentityLabel(String identityId, String label) async {
    // In production, store label mapping
  }
}