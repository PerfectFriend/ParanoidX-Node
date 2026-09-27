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
    required this.id,
    required this.from,
    required this.text,
    required this.timestamp,
    required this.isUser,
    this.chatId = '',
    this.status = '',
    this.replyToId,
    this.replyText,
    this.pinned,
    this.reactions,
    this.isForwarded = false,
    this.moneyAmount,
    this.moneyAsset,
    this.moneyTxId,
    this.voiceUrl,
    this.voiceDuration,
    this.fileName,
    this.fileUrl,
    this.fileSize,
    this.readAt,
    this.deliveredAt,
    this.recalled = false,
    this.recalledAt,
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
    try {
      return DateTime.parse(timestamp);
    } catch (_) {
      return DateTime.now();
    }
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

/// ChatStatus represents the status of the chat service
class ChatStatus {
  final String status;
  final int totalContacts;
  final int unreadMessages;
  final String bridgeStatus;

  ChatStatus({
    required this.status,
    required this.totalContacts,
    required this.unreadMessages,
    required this.bridgeStatus,
  });

  factory ChatStatus.fromJson(Map<String, dynamic> json) {
    return ChatStatus(
      status: json['status'] as String? ?? 'unknown',
      totalContacts: json['total_contacts'] as int? ?? 0,
      unreadMessages: json['unread_messages'] as int? ?? 0,
      bridgeStatus: json['bridge_status'] as String? ?? 'unknown',
    );
  }
}

/// Conversation represents a chat conversation
class Conversation {
  final String id;
  final String name;
  final String description;
  final List<String> participants;
  final String createdBy;
  final int createdAt;
  final String? avatarUrl;
  final bool isPrivate;

  Conversation({
    required this.id,
    required this.name,
    required this.description,
    required this.participants,
    required this.createdBy,
    required this.createdAt,
    this.avatarUrl,
    this.isPrivate = true,
  });

  factory Conversation.fromJson(Map<String, dynamic> json) {
    return Conversation(
      id: json['id'] as String? ?? '',
      name: json['name'] as String? ?? '',
      description: json['description'] as String? ?? '',
      participants: (json['participants'] as List<dynamic>?)?.cast<String>() ?? [],
      createdBy: json['created_by'] as String? ?? '',
      createdAt: json['created_at'] as int? ?? 0,
      avatarUrl: json['avatar_url'] as String?,
      isPrivate: json['is_private'] as bool? ?? true,
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'id': id,
      'name': name,
      'description': description,
      'participants': participants,
      'created_by': createdBy,
      'created_at': createdAt,
      'avatar_url': avatarUrl,
      'is_private': isPrivate,
    };
  }
}

/// MessageResult represents the result of sending a message
class MessageResult {
  final String id;
  final String status;
  final String? txId;

  MessageResult({required this.id, required this.status, this.txId});

  factory MessageResult.fromJson(Map<String, dynamic> json) {
    return MessageResult(
      id: json['id'] as String? ?? '',
      status: json['status'] as String? ?? 'sent',
      txId: json['tx_id'] as String?,
    );
  }
}

/// BroadcastResult represents the result of a broadcast message
class BroadcastResult {
  final String id;
  final String status;
  final int recipients;

  BroadcastResult({required this.id, required this.status, required this.recipients});

  factory BroadcastResult.fromJson(Map<String, dynamic> json) {
    return BroadcastResult(
      id: json['id'] as String? ?? '',
      status: json['status'] as String? ?? 'sent',
      recipients: json['recipients'] as int? ?? 0,
    );
  }
}

/// TreasuryAlertResult represents the result of a treasury alert
class TreasuryAlertResult {
  final String id;
  final String status;
  final String message;

  TreasuryAlertResult({required this.id, required this.status, required this.message});

  factory TreasuryAlertResult.fromJson(Map<String, dynamic> json) {
    return TreasuryAlertResult(
      id: json['id'] as String? ?? '',
      status: json['status'] as String? ?? 'sent',
      message: json['message'] as String? ?? '',
    );
  }
}

/// ChatMessageModel is an alias for ChatMessage in models
typedef ChatMessageModel = ChatMessage;