import 'package:intl/intl.dart';

/// ChatMessage manages data model for a single chat message with all metadata.
class ChatMessage {
  final String id;
  final String from;
  final String text;
  final String timestamp;
  final bool isUser;
  final String chatId;
  final String status;
  final String? replyToId;
  final String? replyText;
  final String? pinned;
  final Map<String, int>? reactions;
  final bool isForwarded;

  final double? moneyAmount;
  final String? moneyAsset;
  final String? moneyTxId;
  final String? voiceUrl;
  final int? voiceDuration;
  final String? fileName;
  final String? fileUrl;
  final int? fileSize;
  final String? readAt;
  final String? deliveredAt;
  final bool recalled;
  final String? recalledAt;
  final String? encryption;

  ChatMessage({
    required this.id, required this.from, required this.text,
    required this.timestamp, required this.isUser, this.chatId = '',
    this.status = '', this.replyToId, this.replyText, this.pinned,
    this.reactions, this.isForwarded = false,
    this.moneyAmount, this.moneyAsset, this.moneyTxId,
    this.voiceUrl, this.voiceDuration,
    this.fileName, this.fileUrl, this.fileSize,
    this.readAt, this.deliveredAt,
    this.recalled = false, this.recalledAt,
    this.encryption,
  });

  factory ChatMessage.fromJson(Map<String, dynamic> json) {
    Map<String, int>? rx;
    if (json['reactions'] != null) {
      rx = (json['reactions'] as Map<String, dynamic>)
          .map((k, v) => MapEntry(k, (v as num).toInt()));
    }
    return ChatMessage(
      id: json['id'] as String? ?? '',
      from: json['from'] as String? ?? '',
      text: json['text'] as String? ?? '',
      timestamp: json['timestamp'] as String? ?? '',
      isUser: json['is_user'] as bool? ?? false,
      chatId: json['chat_id'] as String? ?? '',
      status: json['status'] as String? ?? '',
      replyToId: json['reply_to_id'] as String?,
      replyText: json['reply_text'] as String?,
      pinned: json['pinned'] as String?,
      reactions: rx,
      isForwarded: json['is_forwarded'] as bool? ?? false,
      moneyAmount: (json['money_amount'] as num?)?.toDouble(),
      moneyAsset: json['money_asset'] as String?,
      moneyTxId: json['money_tx_id'] as String?,
      voiceUrl: json['voice_url'] as String?,
      voiceDuration: json['voice_duration'] as int?,
      fileName: json['file_name'] as String?,
      fileUrl: json['file_url'] as String?,
      fileSize: json['file_size'] as int?,
      readAt: json['read_at'] as String?,
      deliveredAt: json['delivered_at'] as String?,
      recalled: json['recalled'] as bool? ?? false,
      recalledAt: json['recalled_at'] as String?,
      encryption: json['encryption'] as String?,
    );
  }

  DateTime get time {
    try { return DateTime.parse(timestamp); } catch (_) { return DateTime.now(); }
  }

  String get displayTime {
    final t = time;
    final now = DateTime.now();
    if (t.day == now.day && t.month == now.month && t.year == now.year) {
      return _timeFmt.format(t);
    }
    return _dateFmt.format(t);
  }

  static final _timeFmt = DateFormat('HH:mm');
  static final _dateFmt = DateFormat('yyyy-MM-dd HH:mm');
}
