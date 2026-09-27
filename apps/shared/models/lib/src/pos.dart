/// Invoice represents a POS invoice
class Invoice {
  final String id;
  final String merchant;
  final int amountNg;
  final String? description;
  final String status;
  final String createdAt;
  final String? paidAt;
  final bool paid;

  Invoice({
    required this.id,
    required this.merchant,
    required this.amountNg,
    this.description,
    required this.status,
    required this.createdAt,
    this.paidAt,
    required this.paid,
  });

  factory Invoice.fromJson(Map<String, dynamic> json) {
    return Invoice(
      id: json['id'] as String? ?? '',
      merchant: json['merchant'] as String? ?? '',
      amountNg: json['amount_ng'] as int? ?? 0,
      description: json['description'] as String?,
      status: json['status'] as String? ?? 'pending',
      createdAt: json['created_at'] as String? ?? '',
      paidAt: json['paid_at'] as String?,
      paid: json['paid'] as bool? ?? false,
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'id': id,
      'merchant': merchant,
      'amount_ng': amountNg,
      'description': description,
      'status': status,
      'created_at': createdAt,
      'paid_at': paidAt,
      'paid': paid,
    };
  }
}

/// PaymentResult represents the result of a payment
class PaymentResult {
  final String transactionId;
  final String status;
  final String message;

  PaymentResult({required this.transactionId, required this.status, required this.message});

  factory PaymentResult.fromJson(Map<String, dynamic> json) {
    return PaymentResult(
      transactionId: json['transaction_id'] as String? ?? '',
      status: json['status'] as String? ?? 'unknown',
      message: json['message'] as String? ?? '',
    );
  }
}