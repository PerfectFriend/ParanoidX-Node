/// Banknote manages banknote functionality.
class Banknote {
  final String serial;
  final int denominationNg;
  final String rarity;
  final String holder;
  final int frozenNg;
  final String status;
  final bool isGolden;
  final String? pdfHash;

  const Banknote({
    required this.serial,
    required this.denominationNg,
    required this.rarity,
    required this.holder,
    this.frozenNg = 0,
    this.status = 'active',
    this.isGolden = false,
    this.pdfHash,
  });

  factory Banknote.fromJson(Map<String, dynamic> json) {
    return Banknote(
      serial: json['serial'] ?? '',
      denominationNg: (json['denomination_ng'] ?? 0).toInt(),
      rarity: json['rarity'] ?? 'common',
      holder: json['holder'] ?? '',
      frozenNg: (json['frozen_ng'] ?? 0).toInt(),
      status: json['status'] ?? 'active',
      isGolden: json['is_golden'] ?? false,
      pdfHash: json['pdf_hash'],
    );
  }

  String get rarityLabel {
    switch (rarity) {
      case 'common': return 'Common';
      case 'rare': return 'Rare';
      case 'epic': return 'Epic';
      case 'legendary': return 'Legendary';
      case 'genesis': return 'Genesis';
      default: return rarity;
    }
  }
}
