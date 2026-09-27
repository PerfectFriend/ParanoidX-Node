class ExternalWallet {
  final String id;
  final String pubkey;
  final String walletType;
  final String walletAddress;
  final String label;
  final String chain;
  final bool isVerified;
  final DateTime createdAt;
  final DateTime? lastSyncAt;

  const ExternalWallet({
    required this.id,
    required this.pubkey,
    required this.walletType,
    required this.walletAddress,
    required this.label,
    required this.chain,
    this.isVerified = false,
    required this.createdAt,
    this.lastSyncAt,
  });

  factory ExternalWallet.fromJson(Map<String, dynamic> json) => ExternalWallet(
        id: json['id'] ?? '',
        pubkey: json['pubkey'] ?? '',
        walletType: json['wallet_type'] ?? '',
        walletAddress: json['wallet_address'] ?? '',
        label: json['label'] ?? '',
        chain: json['chain'] ?? 'all',
        isVerified: json['is_verified'] == true || json['is_verified'] == 1,
        createdAt: json['created_at'] != null
            ? DateTime.tryParse(json['created_at']) ?? DateTime.now()
            : DateTime.now(),
        lastSyncAt: json['last_sync_at'] != null
            ? DateTime.tryParse(json['last_sync_at'])
            : null,
      );

  Map<String, dynamic> toJson() => {
        'id': id,
        'pubkey': pubkey,
        'wallet_type': walletType,
        'wallet_address': walletAddress,
        'label': label,
        'chain': chain,
        'is_verified': isVerified,
        'created_at': createdAt.toIso8601String(),
        if (lastSyncAt != null) 'last_sync_at': lastSyncAt!.toIso8601String(),
      };

  String get displayName =>
      label.isNotEmpty ? label : walletType;
}
