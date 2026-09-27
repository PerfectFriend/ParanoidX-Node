/// WalletBalance represents wallet balance
class WalletBalance {
  final int totalNg;
  final int availableNg;
  final int reservedNg;
  final int stakedNg;
  final int silverNg;
  final double totalValueUsd;

  WalletBalance({
    required this.totalNg,
    required this.availableNg,
    required this.reservedNg,
    required this.stakedNg,
    required this.silverNg,
    required this.totalValueUsd,
  });

  factory WalletBalance.fromJson(Map<String, dynamic> json) {
    return WalletBalance(
      totalNg: (json['total_ng'] as num?)?.toInt() ?? 0,
      availableNg: (json['available_ng'] as num?)?.toInt() ?? 0,
      reservedNg: (json['reserved_ng'] as num?)?.toInt() ?? 0,
      stakedNg: (json['staked_ng'] as num?)?.toInt() ?? 0,
      silverNg: (json['silver_ng'] as num?)?.toInt() ?? 0,
      totalValueUsd: (json['total_value_usd'] as num?)?.toDouble() ?? 0.0,
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'total_ng': totalNg,
      'available_ng': availableNg,
      'reserved_ng': reservedNg,
      'staked_ng': stakedNg,
      'silver_ng': silverNg,
      'total_value_usd': totalValueUsd,
    };
  }
}

/// WalletTransaction represents a wallet transaction
class WalletTransaction {
  final String id;
  final String type;
  final int amountNg;
  final String? fromPubkey;
  final String? toPubkey;
  final String? memo;
  final String status;
  final String createdAt;

  WalletTransaction({
    required this.id,
    required this.type,
    required this.amountNg,
    this.fromPubkey,
    this.toPubkey,
    this.memo,
    required this.status,
    required this.createdAt,
  });

  factory WalletTransaction.fromJson(Map<String, dynamic> json) {
    return WalletTransaction(
      id: json['id'] as String? ?? '',
      type: json['type'] as String? ?? '',
      amountNg: (json['amount_ng'] as num?)?.toInt() ?? 0,
      fromPubkey: json['from_pubkey'] as String?,
      toPubkey: json['to_pubkey'] as String?,
      memo: json['memo'] as String?,
      status: json['status'] as String? ?? 'pending',
      createdAt: json['created_at'] as String? ?? '',
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'id': id,
      'type': type,
      'amount_ng': amountNg,
      'from_pubkey': fromPubkey,
      'to_pubkey': toPubkey,
      'memo': memo,
      'status': status,
      'created_at': createdAt,
    };
  }
}

/// BanknoteResult represents the result of banknote operations
class BanknoteResult {
  final String banknoteId;
  final String status;
  final String message;

  BanknoteResult({required this.banknoteId, required this.status, required this.message});

  factory BanknoteResult.fromJson(Map<String, dynamic> json) {
    return BanknoteResult(
      banknoteId: json['banknote_id'] as String? ?? '',
      status: json['status'] as String? ?? 'unknown',
      message: json['message'] as String? ?? '',
    );
  }
}

/// MintResult represents the result of minting
class MintResult {
  final String banknoteId;
  final int amountNg;
  final String status;
  final String message;

  MintResult({required this.banknoteId, required this.amountNg, required this.status, required this.message});

  factory MintResult.fromJson(Map<String, dynamic> json) {
    return MintResult(
      banknoteId: json['banknote_id'] as String? ?? '',
      amountNg: (json['amount_ng'] as num?)?.toInt() ?? 0,
      status: json['status'] as String? ?? 'unknown',
      message: json['message'] as String? ?? '',
    );
  }
}

/// RedeemResult represents the result of redeeming
class RedeemResult {
  final String status;
  final String message;
  final int amountNg;

  RedeemResult({required this.status, required this.message, required this.amountNg});

  factory RedeemResult.fromJson(Map<String, dynamic> json) {
    return RedeemResult(
      status: json['status'] as String? ?? 'unknown',
      message: json['message'] as String? ?? '',
      amountNg: (json['amount_ng'] as num?)?.toInt() ?? 0,
    );
  }
}

/// DividendResult represents the result of dividend claim
class DividendResult {
  final String status;
  final String message;
  final int amountNg;

  DividendResult({required this.status, required this.message, required this.amountNg});

  factory DividendResult.fromJson(Map<String, dynamic> json) {
    return DividendResult(
      status: json['status'] as String? ?? 'unknown',
      message: json['message'] as String? ?? '',
      amountNg: (json['amount_ng'] as num?)?.toInt() ?? 0,
    );
  }
}

/// TransferResult represents the result of a transfer
class TransferResult {
  final String transactionId;
  final String status;
  final String message;

  TransferResult({required this.transactionId, required this.status, required this.message});

  factory TransferResult.fromJson(Map<String, dynamic> json) {
    return TransferResult(
      transactionId: json['transaction_id'] as String? ?? '',
      status: json['status'] as String? ?? 'unknown',
      message: json['message'] as String? ?? '',
    );
  }
}

/// ReceiveInfo represents receive information
class ReceiveInfo {
  final String address;
  final String? qrCode;
  final String? uri;

  ReceiveInfo({required this.address, this.qrCode, this.uri});

  factory ReceiveInfo.fromJson(Map<String, dynamic> json) {
    return ReceiveInfo(
      address: json['address'] as String? ?? '',
      qrCode: json['qr_code'] as String?,
      uri: json['uri'] as String?,
    );
  }
}