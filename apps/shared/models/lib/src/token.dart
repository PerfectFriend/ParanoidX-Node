class Token {
  final String symbol;
  final String name;
  final int decimals;
  final String chain;
  final String? contractAddress;
  final String? logoUrl;
  final bool isCustom;
  final DateTime createdAt;

  const Token({
    required this.symbol,
    required this.name,
    required this.decimals,
    required this.chain,
    this.contractAddress,
    this.logoUrl,
    this.isCustom = false,
    required this.createdAt,
  });

  factory Token.fromJson(Map<String, dynamic> json) => Token(
        symbol: json['symbol'] ?? '',
        name: json['name'] ?? '',
        decimals: (json['decimals'] ?? 0) as int,
        chain: json['chain'] ?? '',
        contractAddress: json['contract_address'] as String?,
        logoUrl: json['logo_url'] as String?,
        isCustom: json['is_custom'] == true || json['is_custom'] == 1,
        createdAt: json['created_at'] != null
            ? DateTime.tryParse(json['created_at']) ?? DateTime.now()
            : DateTime.now(),
      );

  Map<String, dynamic> toJson() => {
        'symbol': symbol,
        'name': name,
        'decimals': decimals,
        'chain': chain,
        if (contractAddress != null) 'contract_address': contractAddress,
        if (logoUrl != null) 'logo_url': logoUrl,
        'is_custom': isCustom,
        'created_at': createdAt.toIso8601String(),
      };
}

class TokenBalance {
  final String symbol;
  final String name;
  final String balance;
  final String? updatedAt;

  const TokenBalance({
    required this.symbol,
    required this.name,
    required this.balance,
    this.updatedAt,
  });

  factory TokenBalance.fromJson(Map<String, dynamic> json) => TokenBalance(
        symbol: json['symbol'] ?? '',
        name: json['name'] ?? '',
        balance: json['balance'] ?? '0',
        updatedAt: json['updated_at'] as String?,
      );

  double get balanceValue => double.tryParse(balance) ?? 0;

  String get formattedBalance {
    final v = balanceValue;
    if (v >= 1000000) return '${(v / 1000000).toStringAsFixed(2)}M';
    if (v >= 1000) return '${(v / 1000).toStringAsFixed(2)}K';
    return v.toStringAsFixed(4);
  }
}

/// TokenBalancesResponse represents token balance query response
class TokenBalancesResponse {
  final List<TokenBalance> balances;
  TokenBalancesResponse({this.balances = const []});
  factory TokenBalancesResponse.fromJson(Map<String, dynamic> json) => TokenBalancesResponse(
    balances: (json['balances'] as List? ?? []).map((e) => TokenBalance.fromJson(e)).toList(),
  );
}

/// TokenOperationResult represents token operation result (add/remove custom)
class TokenOperationResult {
  final bool success;
  final String message;
  TokenOperationResult({this.success = false, this.message = ''});
  factory TokenOperationResult.fromJson(Map<String, dynamic> json) => TokenOperationResult(
    success: json['success'] ?? false,
    message: json['message'] ?? '',
  );
}
