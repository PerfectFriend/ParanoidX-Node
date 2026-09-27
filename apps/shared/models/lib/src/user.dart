/// AppUser manages appuser functionality.
class AppUser {
  final String pubkey;
  final String? privkey;
  final String? mnemonic;
  final int balanceNg;
  final int frozenNg;
  final int banknoteCount;
  final String reputationTier;

  const AppUser({
    required this.pubkey,
    this.privkey,
    this.mnemonic,
    this.balanceNg = 0,
    this.frozenNg = 0,
    this.banknoteCount = 0,
    this.reputationTier = 'basic',
  });

  factory AppUser.fromJson(Map<String, dynamic> json) {
    return AppUser(
      pubkey: json['pubkey'] ?? '',
      privkey: json['privkey'],
      mnemonic: json['mnemonic'],
      balanceNg: (json['liquid_balance_ng'] ?? 0).toInt(),
      frozenNg: (json['frozen_ng'] ?? 0).toInt(),
      banknoteCount: (json['banknotes_count'] ?? 0).toInt(),
      reputationTier: json['reputation_tier'] ?? 'basic',
    );
  }

  Map<String, dynamic> toJson() => {
    'pubkey': pubkey,
    'liquid_balance_ng': balanceNg,
    'frozen_ng': frozenNg,
    'banknotes_count': banknoteCount,
  };
}
