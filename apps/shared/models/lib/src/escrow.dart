/// Escrow manages escrow functionality.
class Escrow {
  final String id;
  final String buyer;
  final String seller;
  final String itemId;
  final int priceNg;
  final String status;
  final String createdAt;

  const Escrow({
    required this.id,
    required this.buyer,
    required this.seller,
    required this.itemId,
    required this.priceNg,
    required this.status,
    required this.createdAt,
  });

  factory Escrow.fromJson(Map<String, dynamic> json) {
    return Escrow(
      id: json['id'] ?? '',
      buyer: json['buyer'] ?? '',
      seller: json['seller'] ?? '',
      itemId: json['item_id'] ?? '',
      priceNg: (json['price_ng'] ?? 0).toInt(),
      status: json['status'] ?? 'active',
      createdAt: json['created_at'] ?? '',
    );
  }
}
