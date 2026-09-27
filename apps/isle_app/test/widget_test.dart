import 'package:flutter_test/flutter_test.dart';
import 'package:isle_app/models/chat_message.dart';

void main() {
  test('ChatMessage model serialization', () {
    final msg = ChatMessage(
      id: 'test1',
      from: 'user',
      text: 'hello',
      timestamp: '2026-07-05T12:00:00Z',
      isUser: true,
    );
    expect(msg.id, 'test1');
    expect(msg.text, 'hello');
    expect(msg.isUser, true);
  });

  test('ChatMessage money transfer fields', () {
    final msg = ChatMessage(
      id: 'pay1',
      from: 'contact',
      text: 'payment',
      timestamp: '2026-07-05T12:00:00Z',
      isUser: false,
      moneyAmount: 100,
      moneyAsset: 'XAG',
      moneyTxId: 'tx123',
    );
    expect(msg.moneyAmount, 100);
    expect(msg.moneyAsset, 'XAG');
    expect(msg.moneyTxId, 'tx123');
  });

  test('ChatMessage encryption field', () {
    final msg = ChatMessage(
      id: 'e2e1',
      from: 'user',
      text: 'secret',
      timestamp: '2026-07-05T12:00:00Z',
      isUser: true,
      encryption: 'e2e',
    );
    expect(msg.encryption, 'e2e');
  });

  test('ChatMessage reply and forward', () {
    final msg = ChatMessage(
      id: 'r1',
      from: 'user',
      text: 'reply',
      timestamp: '2026-07-05T12:00:00Z',
      isUser: true,
      replyToId: 'orig1',
      replyText: 'original message',
      isForwarded: true,
    );
    expect(msg.replyToId, 'orig1');
    expect(msg.replyText, 'original message');
    expect(msg.isForwarded, true);
  });

  test('ChatMessage read receipts and recall', () {
    final msg = ChatMessage(
      id: 'rr1',
      from: 'contact',
      text: 'read this',
      timestamp: '2026-07-05T12:00:00Z',
      isUser: false,
      deliveredAt: '2026-07-05T12:00:05Z',
      readAt: '2026-07-05T12:00:10Z',
      recalled: true,
      recalledAt: '2026-07-05T12:01:00Z',
    );
    expect(msg.deliveredAt, '2026-07-05T12:00:05Z');
    expect(msg.readAt, '2026-07-05T12:00:10Z');
    expect(msg.recalled, true);
  });
}
