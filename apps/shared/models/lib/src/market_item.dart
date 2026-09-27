/// MarketItem manages marketitem functionality.
class MarketItem {
  final String id;
  final String name;
  final int priceNg;
  final bool forSale;
  final String? holder;

  const MarketItem({
    required this.id,
    required this.name,
    required this.priceNg,
    required this.forSale,
    this.holder,
  });

  factory MarketItem.fromJson(Map<String, dynamic> json) {
    return MarketItem(
      id: json['id'] as String? ?? '',
      name: json['name'] as String? ?? '',
      priceNg: (json['price_ng'] as num?)?.toInt() ?? 0,
      forSale: json['for_sale'] as bool? ?? false,
      holder: json['holder'] as String?,
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'id': id,
      'name': name,
      'price_ng': priceNg,
      'for_sale': forSale,
      'holder': holder,
    };
  }
}

/// MarketListing represents a listing on the marketplace
class MarketListing {
  final String id;
  final String itemId;
  final String seller;
  final int priceNg;
  final int quantity;
  final String status;
  final String createdAt;
  final String serial;
  final int denominationNg;
  final String grade;
  final bool available;

  MarketListing({
    this.id = '',
    this.itemId = '',
    required this.seller,
    required this.priceNg,
    this.quantity = 1,
    this.status = 'active',
    this.createdAt = '',
    required this.serial,
    required this.denominationNg,
    required this.grade,
    this.available = true,
  });

  factory MarketListing.fromJson(Map<String, dynamic> json) {
    return MarketListing(
      id: json['id'] as String? ?? '',
      itemId: json['item_id'] as String? ?? '',
      seller: json['seller'] as String? ?? '',
      priceNg: (json['price_ng'] as num?)?.toInt() ?? 0,
      quantity: (json['quantity'] as num?)?.toInt() ?? 0,
      status: json['status'] as String? ?? 'active',
      createdAt: json['created_at'] as String? ?? '',
      serial: json['serial'] as String? ?? '',
      denominationNg: (json['denomination_ng'] as num?)?.toInt() ?? 0,
      grade: json['grade'] as String? ?? 'common',
      available: json['available'] as bool? ?? true,
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'id': id,
      'item_id': itemId,
      'seller': seller,
      'price_ng': priceNg,
      'quantity': quantity,
      'status': status,
      'created_at': createdAt,
      'serial': serial,
      'denomination_ng': denominationNg,
      'grade': grade,
      'available': available,
    };
  }
}

/// MarketOrder represents a market order
class MarketOrder {
  final String id;
  final String listingId;
  final String buyer;
  final String seller;
  final int priceNg;
  final int quantity;
  final String status;
  final String createdAt;
  final int amountNg;

  MarketOrder({
    required this.id,
    required this.listingId,
    required this.buyer,
    required this.seller,
    required this.priceNg,
    required this.quantity,
    required this.status,
    required this.createdAt,
    required this.amountNg,
  });

  factory MarketOrder.fromJson(Map<String, dynamic> json) {
    return MarketOrder(
      id: json['id'] as String? ?? '',
      listingId: json['listing_id'] as String? ?? '',
      buyer: json['buyer'] as String? ?? '',
      seller: json['seller'] as String? ?? '',
      priceNg: (json['price_ng'] as num?)?.toInt() ?? 0,
      quantity: (json['quantity'] as num?)?.toInt() ?? 0,
      status: json['status'] as String? ?? 'pending',
      createdAt: json['created_at'] as String? ?? '',
      amountNg: (json['amount_ng'] as num?)?.toInt() ?? 0,
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'id': id,
      'listing_id': listingId,
      'buyer': buyer,
      'seller': seller,
      'price_ng': priceNg,
      'quantity': quantity,
      'status': status,
      'created_at': createdAt,
      'amount_ng': amountNg,
    };
  }
}

/// PurchaseResult represents the result of a purchase
class PurchaseResult {
  final String orderId;
  final String status;
  final String message;

  PurchaseResult({required this.orderId, required this.status, required this.message});

  factory PurchaseResult.fromJson(Map<String, dynamic> json) {
    return PurchaseResult(
      orderId: json['order_id'] as String? ?? '',
      status: json['status'] as String? ?? 'unknown',
      message: json['message'] as String? ?? '',
    );
  }
}

/// EscrowResult represents the result of escrow creation
class EscrowResult {
  final String escrowId;
  final String status;
  final String message;

  EscrowResult({required this.escrowId, required this.status, required this.message});

  factory EscrowResult.fromJson(Map<String, dynamic> json) {
    return EscrowResult(
      escrowId: json['escrow_id'] as String? ?? '',
      status: json['status'] as String? ?? 'unknown',
      message: json['message'] as String? ?? '',
    );
  }
}