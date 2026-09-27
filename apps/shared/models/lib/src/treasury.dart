/// Treasury and economy models for the Royal Admin interface

/// OraclePrice represents the oracle price data
class OraclePrice {
  final double price;
  final double change24h;

  OraclePrice({required this.price, required this.change24h});

  factory OraclePrice.fromJson(Map<String, dynamic> json) {
    return OraclePrice(
      price: (json['current_price'] as num?)?.toDouble() ?? (json['price'] as num?)?.toDouble() ?? 0.0,
      change24h: (json['change_24h'] as num?)?.toDouble() ?? 0.0,
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'price': price,
      'change_24h': change24h,
    };
  }
}

/// DeflationState represents the deflation state
class DeflationState {
  final bool enabled;
  final int intervalMs;
  final double rate;

  DeflationState({required this.enabled, required this.intervalMs, required this.rate});

  factory DeflationState.fromJson(Map<String, dynamic> json) {
    return DeflationState(
      enabled: json['enabled'] as bool? ?? false,
      intervalMs: (json['interval_ms'] as num?)?.toInt() ?? 0,
      rate: (json['rate'] as num?)?.toDouble() ?? 0.0,
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'enabled': enabled,
      'interval_ms': intervalMs,
      'rate': rate,
    };
  }
}

/// AutoMintConfig represents the auto-mint configuration
class AutoMintConfig {
  final bool enabled;
  final int intervalMs;

  AutoMintConfig({required this.enabled, required this.intervalMs});

  factory AutoMintConfig.fromJson(Map<String, dynamic> json) {
    return AutoMintConfig(
      enabled: json['enabled'] as bool? ?? false,
      intervalMs: (json['interval_ms'] as num?)?.toInt() ?? 0,
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'enabled': enabled,
      'interval_ms': intervalMs,
    };
  }
}

/// DividendPool represents the dividend pool state
class DividendPool {
  final int balanceNg;
  final int distributedNg;
  final String lastDistribution;

  DividendPool({required this.balanceNg, required this.distributedNg, required this.lastDistribution});

  factory DividendPool.fromJson(Map<String, dynamic> json) {
    return DividendPool(
      balanceNg: (json['balance_ng'] as num?)?.toInt() ?? 0,
      distributedNg: (json['distributed_ng'] as num?)?.toInt() ?? 0,
      lastDistribution: json['last_distribution'] as String? ?? '',
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'balance_ng': balanceNg,
      'distributed_ng': distributedNg,
      'last_distribution': lastDistribution,
    };
  }
}

/// DividendHistory represents a dividend history entry
class DividendHistory {
  final int amountNg;
  final int recipients;
  final String timestamp;

  DividendHistory({required this.amountNg, required this.recipients, required this.timestamp});

  factory DividendHistory.fromJson(Map<String, dynamic> json) {
    return DividendHistory(
      amountNg: (json['amount_ng'] as num?)?.toInt() ?? 0,
      recipients: (json['recipients'] as num?)?.toInt() ?? 0,
      timestamp: json['timestamp'] as String? ?? '',
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'amount_ng': amountNg,
      'recipients': recipients,
      'timestamp': timestamp,
    };
  }
}

/// SystemStatus represents overall system health status
class SystemStatus {
  final bool healthy;
  final int uptimeHours;
  final String version;
  SystemStatus({this.healthy = false, this.uptimeHours = 0, this.version = ''});
  factory SystemStatus.fromJson(Map<String, dynamic> json) => SystemStatus(
    healthy: json['healthy'] ?? false,
    uptimeHours: (json['uptime_hours'] ?? 0).toInt(),
    version: json['version'] ?? '',
  );
}

/// ReserveState represents reserve state
class ReserveState {
  final int reserveNg;
  final int supplyNg;
  final double backingRatio;
  ReserveState({this.reserveNg = 0, this.supplyNg = 0, this.backingRatio = 0.0});
  factory ReserveState.fromJson(Map<String, dynamic> json) => ReserveState(
    reserveNg: (json['reserve_ng'] ?? 0).toInt(),
    supplyNg: (json['supply_ng'] ?? 0).toInt(),
    backingRatio: (json['backing_ratio'] ?? 0.0).toDouble(),
  );
}

/// BurnResult represents a burn operation result
class BurnResult {
  final bool success;
  final String assetId;
  final int amountNg;
  final String message;
  BurnResult({this.success = false, this.assetId = '', this.amountNg = 0, this.message = ''});
  factory BurnResult.fromJson(Map<String, dynamic> json) => BurnResult(
    success: json['success'] ?? false,
    assetId: json['asset_id'] ?? '',
    amountNg: (json['amount_ng'] ?? 0).toInt(),
    message: json['message'] ?? '',
  );
}

/// ProofOfReserve represents proof of silver reserve
class ProofOfReserve {
  final bool verified;
  final int reserveNg;
  final int supplyNg;
  final double ratio;
  final String timestamp;
  ProofOfReserve({this.verified = false, this.reserveNg = 0, this.supplyNg = 0, this.ratio = 0.0, this.timestamp = ''});
  factory ProofOfReserve.fromJson(Map<String, dynamic> json) => ProofOfReserve(
    verified: json['verified'] ?? false,
    reserveNg: (json['reserve_ng'] ?? 0).toInt(),
    supplyNg: (json['supply_ng'] ?? 0).toInt(),
    ratio: (json['ratio'] ?? 0.0).toDouble(),
    timestamp: json['timestamp'] ?? '',
  );
}

/// Rates represents current exchange rates
class Rates {
  final double silverUsd;
  final double usdtPerNg;
  Rates({this.silverUsd = 0.0, this.usdtPerNg = 0.0});
  factory Rates.fromJson(Map<String, dynamic> json) => Rates(
    silverUsd: (json['silver_usd'] ?? 0.0).toDouble(),
    usdtPerNg: (json['usdt_per_ng'] ?? 0.0).toDouble(),
  );
}

/// Tokenomics represents tokenomics data
class Tokenomics {
  final int totalSupply;
  final int circulatingSupply;
  final int reserveNg;
  final int dividendPoolNg;
  final double backingRatio;
  Tokenomics({this.totalSupply = 0, this.circulatingSupply = 0, this.reserveNg = 0, this.dividendPoolNg = 0, this.backingRatio = 0.0});
  factory Tokenomics.fromJson(Map<String, dynamic> json) => Tokenomics(
    totalSupply: (json['total_supply'] ?? 0).toInt(),
    circulatingSupply: (json['circulating_supply'] ?? 0).toInt(),
    reserveNg: (json['reserve_ng'] ?? 0).toInt(),
    dividendPoolNg: (json['dividend_pool_ng'] ?? 0).toInt(),
    backingRatio: (json['backing_ratio'] ?? 0.0).toDouble(),
  );
}

/// Forecast represents economic forecast
class Forecast {
  final double projectedPrice;
  final double projectedReserve;
  final double confidence;
  final String period;
  Forecast({this.projectedPrice = 0.0, this.projectedReserve = 0.0, this.confidence = 0.0, this.period = ''});
  factory Forecast.fromJson(Map<String, dynamic> json) => Forecast(
    projectedPrice: (json['projected_price'] ?? 0.0).toDouble(),
    projectedReserve: (json['projected_reserve'] ?? 0.0).toDouble(),
    confidence: (json['confidence'] ?? 0.0).toDouble(),
    period: json['period'] ?? '',
  );
}

/// Constitution represents the island constitution
class Constitution {
  final String preamble;
  final List<Map<String, dynamic>> articles;
  Constitution({this.preamble = '', this.articles = const []});
  factory Constitution.fromJson(Map<String, dynamic> json) => Constitution(
    preamble: json['preamble'] ?? '',
    articles: (json['articles'] as List? ?? []).map((e) => Map<String, dynamic>.from(e)).toList(),
  );
}

/// Proposal represents a governance proposal
class Proposal {
  final String id;
  final String title;
  final String description;
  final String proposer;
  final String status;
  final int yesVotes;
  final int noVotes;
  final String deadline;
  final String createdAt;
  Proposal({this.id = '', this.title = '', this.description = '', this.proposer = '', this.status = '', this.yesVotes = 0, this.noVotes = 0, this.deadline = '', this.createdAt = ''});
  factory Proposal.fromJson(Map<String, dynamic> json) => Proposal(
    id: json['id'] ?? '',
    title: json['title'] ?? '',
    description: json['description'] ?? '',
    proposer: json['proposer'] ?? '',
    status: json['status'] ?? '',
    yesVotes: (json['yes_votes'] ?? 0).toInt(),
    noVotes: (json['no_votes'] ?? 0).toInt(),
    deadline: json['deadline'] ?? '',
    createdAt: json['created_at'] ?? '',
  );
}

/// ProposalDraft represents a governance proposal draft before submission
class ProposalDraft {
  final String title;
  final String description;
  ProposalDraft({required this.title, required this.description});
  Map<String, dynamic> toJson() => {'title': title, 'description': description};
}

/// VoteResult represents the result of a vote
class VoteResult {
  final bool success;
  final bool approved;
  final String message;
  VoteResult({this.success = false, this.approved = false, this.message = ''});
  factory VoteResult.fromJson(Map<String, dynamic> json) => VoteResult(
    success: json['success'] ?? false,
    approved: json['approved'] ?? false,
    message: json['message'] ?? '',
  );
}

/// DelegationResult represents the result of a delegation
class DelegationResult {
  final bool success;
  final String message;
  DelegationResult({this.success = false, this.message = ''});
  factory DelegationResult.fromJson(Map<String, dynamic> json) => DelegationResult(
    success: json['success'] ?? false,
    message: json['message'] ?? '',
  );
}