/// Identity manages sovereign identity with BIP39/BIP32/BIP44 compliance.
/// 
/// Supports:
/// - 24-word BIP39 English mnemonic generation
/// - BIP39 passphrase (25th word) for plausible deniability
/// - BIP32/BIP44 Ed25519 hierarchical deterministic key derivation
/// - Custom coin type 1337 (SMP - SimpleX Messenger Protocol)
/// - Deterministic derivation path: m/44'/1337'/0'/0'/0' (identity key, all hardened)
/// - X25519 derivation for encryption: m/44'/1337'/0'/1'/0' (all hardened)
library models.src.identity;

import 'dart:convert';
import 'dart:typed_data';

import 'package:bip39/bip39.dart' as bip39;
import 'package:bip32/bip32.dart' as bip32;
import 'package:ed25519_hd_key/ed25519_hd_key.dart' as ed25519_hd_key;
import 'package:pinenacl/ed25519.dart' as pinenacl_ed25519;
import 'package:crypto/crypto.dart';
import 'package:convert/convert.dart' show hex;

/// Word count options for BIP39 mnemonic
enum MnemonicWordCount {
  words12(12),
  words15(15),
  words18(18),
  words21(21),
  words24(24);

  const MnemonicWordCount(this.value);
  final int value;

  int get entropyBits => value * 11 - 4; // BIP39 formula
}

/// Identity represents a sovereign identity derived from BIP39 mnemonic.
/// 
/// The identity includes:
/// - Encrypted mnemonic (AES-256-GCM with device-derived key)
/// - Derivation path used
/// - Ed25519 public key for signing (identity)
/// - X25519 public key for encryption (key agreement)
/// - Creation timestamp
class Identity {
  /// Unique identifier for this identity (SHA256 of public key)
  final String id;

  /// Encrypted mnemonic phrase (AES-256-GCM)
  final String encryptedMnemonic;

  /// Derivation path used (default: m/44'/1337'/0'/0/0)
  final String derivationPath;

  /// Ed25519 public key (32 bytes, hex encoded) - for signing/identity
  final String ed25519PubKey;

  /// X25519 public key (32 bytes, hex encoded) - for encryption
  final String x25519PubKey;

  /// Creation timestamp (milliseconds since epoch)
  final int createdAt;

  /// Optional label for user identification
  final String? label;

  /// BIP39 word count used
  final MnemonicWordCount wordCount;

  const Identity({
    required this.id,
    required this.encryptedMnemonic,
    required this.derivationPath,
    required this.ed25519PubKey,
    required this.x25519PubKey,
    required this.createdAt,
    this.label,
    this.wordCount = MnemonicWordCount.words24,
  });

  /// Create Identity from JSON
  factory Identity.fromJson(Map<String, dynamic> json) {
    return Identity(
      id: json['id'] as String,
      encryptedMnemonic: json['encryptedMnemonic'] as String,
      derivationPath: json['derivationPath'] as String,
      ed25519PubKey: json['ed25519PubKey'] as String,
      x25519PubKey: json['x25519PubKey'] as String,
      createdAt: json['createdAt'] as int,
      label: json['label'] as String?,
      wordCount: MnemonicWordCount.values.firstWhere(
        (e) => e.value == (json['wordCount'] as int? ?? 24),
        orElse: () => MnemonicWordCount.words24,
      ),
    );
  }

  /// Convert to JSON for storage
  Map<String, dynamic> toJson() => {
    'id': id,
    'encryptedMnemonic': encryptedMnemonic,
    'derivationPath': derivationPath,
    'ed25519PubKey': ed25519PubKey,
    'x25519PubKey': x25519PubKey,
    'createdAt': createdAt,
    'label': label,
    'wordCount': wordCount.value,
  };

  /// Verify this identity matches a mnemonic phrase
  Future<bool> verifyMnemonic(String mnemonic, {String passphrase = ''}) async {
    try {
      final seed = bip39.mnemonicToSeed(mnemonic, passphrase: passphrase);
      final edKey = await ed25519_hd_key.ED25519_HD_KEY.derivePath(derivationPath, seed);
      final pubKeyHex = hex.encode(edKey.key);
      return pubKeyHex == ed25519PubKey;
    } catch (_) {
      return false;
    }
  }

  /// Get short display ID (first 8 chars)
  String get shortId => id.substring(0, 8);

  /// Get display label or short ID
  String get displayName => label ?? 'Identity $shortId';

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      other is Identity && runtimeType == other.runtimeType && id == other.id;

  @override
  int get hashCode => id.hashCode;

  @override
  String toString() => 'Identity(id: $shortId, label: $label, created: ${DateTime.fromMillisecondsSinceEpoch(createdAt)})';
}

/// KeyPair holds Ed25519 signing keys
class KeyPair {
  final Uint8List privateKey;
  final Uint8List publicKey;

  const KeyPair({
    required this.privateKey,
    required this.publicKey,
  });

  String get privateKeyHex => hex.encode(privateKey);
  String get publicKeyHex => hex.encode(publicKey);

  /// Sign a message
  Uint8List sign(Uint8List message) {
    // Ed25519 signing using pinenacl
    final signingKey = pinenacl_ed25519.SigningKey.fromSeed(privateKey);
    final signedMessage = signingKey.sign(message);
    return signedMessage.signature.asTypedList;
  }

  /// Verify a signature
  bool verify(Uint8List message, Uint8List signature) {
    final verifyKey = pinenacl_ed25519.VerifyKey(publicKey);
    final sig = pinenacl_ed25519.Signature(Uint8List.fromList(signature));
    try {
      return verifyKey.verify(signature: sig, message: message);
    } catch (_) {
      return false;
    }
  }
}

/// EncryptionKeyPair holds X25519 keys for key agreement
class EncryptionKeyPair {
  final Uint8List privateKey;
  final Uint8List publicKey;

  const EncryptionKeyPair({
    required this.privateKey,
    required this.publicKey,
  });

  String get privateKeyHex => hex.encode(privateKey);
  String get publicKeyHex => hex.encode(publicKey);

  /// Perform X25519 key agreement
  Uint8List keyAgreement(Uint8List theirPublicKey) {
    throw UnimplementedError('Use x25519_dart or similar for key agreement');
  }
}

/// DerivedKeys holds all keys derived from a seed
class DerivedKeys {
  final Uint8List seed;
  final KeyPair identityKey;      // Ed25519 for signing (m/44'/1337'/0'/0'/0')
  final EncryptionKeyPair encryptionKey; // X25519 for encryption (m/44'/1337'/0'/1'/0')
  final String mnemonic;
  final String passphrase;

  const DerivedKeys({
    required this.seed,
    required this.identityKey,
    required this.encryptionKey,
    required this.mnemonic,
    required this.passphrase,
  });
}

/// Derivation paths for SMP (coin type 1337)
/// All paths use hardened derivation (') for Ed25519 compatibility
/// since Ed25519 doesn't support public (non-hardened) derivation.
class DerivationPaths {
  static const String identity = "m/44'/1337'/0'/0'/0'";      // Ed25519 signing
  static const String encryption = "m/44'/1337'/0'/1'/0'";    // X25519 encryption
  static const String auth = "m/44'/1337'/0'/2'/0'";          // Ed25519 auth (future)
  static const String storage = "m/44'/1337'/0'/3'/0'";       // Encrypted storage key
}