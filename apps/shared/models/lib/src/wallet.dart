/// TreasuryState represents the treasury state
class TreasuryState {
  final int? reserveNg;
  final int? reserveTlr;
  final int? supplyNg;
  final int? supplyTlr;
  final int? accounts;
  final int? activeBanknotes;
  final int? totalBanknotes;
  final int? rwaCount;
  final String? tier;
  final int? monthlyOpsNg;
  final double? silverSpotUsd;
  final bool? isRoyal;
  final double? silverReserveOz;
  final int? totalNtSupply;
  final double? backingRatio;
  final int? dividendPoolNg;
  final bool? deflationEnabled;
  final int? deflationIntervalMs;
  final double? deflationRate;
  final int? lastDividendAt;

  TreasuryState({
    this.reserveNg,
    this.reserveTlr,
    this.supplyNg,
    this.supplyTlr,
    this.accounts,
    this.activeBanknotes,
    this.totalBanknotes,
    this.rwaCount,
    this.tier,
    this.monthlyOpsNg,
    this.silverSpotUsd,
    this.isRoyal,
    this.silverReserveOz,
    this.totalNtSupply,
    this.backingRatio,
    this.dividendPoolNg,
    this.deflationEnabled,
    this.deflationIntervalMs,
    this.deflationRate,
    this.lastDividendAt,
  });

  factory TreasuryState.fromJson(Map<String, dynamic> json) {
    return TreasuryState(
      reserveNg: (json['reserve_ng'] as num?)?.toInt(),
      reserveTlr: (json['reserve_tlr'] as num?)?.toInt(),
      supplyNg: (json['supply_ng'] as num?)?.toInt(),
      supplyTlr: (json['supply_tlr'] as num?)?.toInt(),
      accounts: (json['accounts'] as num?)?.toInt(),
      activeBanknotes: (json['active_banknotes'] as num?)?.toInt(),
      totalBanknotes: (json['total_banknotes'] as num?)?.toInt(),
      rwaCount: (json['rwa_count'] as num?)?.toInt(),
      tier: json['tier'] as String?,
      monthlyOpsNg: (json['monthly_ops_ng'] as num?)?.toInt(),
      silverSpotUsd: (json['silver_spot_usd'] as num?)?.toDouble(),
      isRoyal: json['is_royal'] as bool?,
      silverReserveOz: (json['silver_reserve_oz'] as num?)?.toDouble(),
      totalNtSupply: (json['total_nt_supply'] as num?)?.toInt(),
      backingRatio: (json['backing_ratio'] as num?)?.toDouble(),
      dividendPoolNg: (json['dividend_pool_ng'] as num?)?.toInt(),
      deflationEnabled: json['deflation_enabled'] as bool?,
      deflationIntervalMs: (json['deflation_interval_ms'] as num?)?.toInt(),
      deflationRate: (json['deflation_rate'] as num?)?.toDouble(),
      lastDividendAt: (json['last_dividend_at'] as num?)?.toInt(),
    );
  }
}