/// BillingEntry manages billingentry functionality.
class BillingEntry {
  final String service;
  final int amountNg;
  final String from;
  final int timestamp;
  final String status;

  const BillingEntry({
    required this.service,
    required this.amountNg,
    required this.from,
    this.timestamp = 0,
    this.status = 'completed',
  });

  factory BillingEntry.fromJson(Map<String, dynamic> json) {
    return BillingEntry(
      service: json['service'] ?? '',
      amountNg: (json['amount_ng'] ?? 0).toInt(),
      from: json['from'] ?? '',
      timestamp: (json['timestamp'] ?? 0).toInt(),
      status: json['status'] ?? 'completed',
    );
  }
}

/// PressTemplate manages presstemplate functionality.
class PressTemplate {
  final String id;
  final String name;
  final String rarity;
  final String description;

  const PressTemplate({
    required this.id,
    required this.name,
    required this.rarity,
    this.description = '',
  });

  factory PressTemplate.fromJson(Map<String, dynamic> json) {
    return PressTemplate(
      id: json['id'] ?? '',
      name: json['name'] ?? '',
      rarity: json['rarity'] ?? 'common',
      description: json['description'] ?? '',
    );
  }
}
