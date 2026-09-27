import 'client.dart';

/// AiClient manages aiclient functionality.
class AiClient {
  final SimplexNodeClient _client;

  AiClient(this._client);

  Future<Map<String, dynamic>> chat(String question, {String? context}) {
    return _client.post('/api/ai/chat', body: {
      'question': question,
      if (context != null) 'context': context,
    });
  }

  Future<Map<String, dynamic>> explainSilver() {
    return _client.post('/api/ai/explain-silver', body: {});
  }

  Future<Map<String, dynamic>> suggestTreasuryAction({
    required int reserveNg,
    required int totalSupply,
    required double recentDeposits,
  }) {
    return _client.post('/api/ai/suggest-treasury', body: {
      'reserve_ng': reserveNg,
      'total_supply': totalSupply,
      'recent_deposits': recentDeposits,
    });
  }

  Future<Map<String, dynamic>> economySummary(String stateJson) {
    return _client.post('/api/ai/economy-summary', body: {
      'state_json': stateJson,
    });
  }

  Future<Map<String, dynamic>> moderationCheck(String text) {
    return _client.post('/api/ai/moderation', body: {
      'text': text,
    });
  }

  Future<Map<String, dynamic>> health() {
    return _client.get('/api/ai/health');
  }
}
