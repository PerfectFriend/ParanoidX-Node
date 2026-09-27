import 'client.dart';

/// PressClient manages pressclient functionality.
class PressClient {
  final SimplexNodeClient _client;
  PressClient(this._client);

  Future<Map<String, dynamic>> listTemplates() =>
      _client.get('/api/press/templates');

  Future<Map<String, dynamic>> issueBanknote({
    required String templateId,
    required String ownerPubkey,
  }) =>
      _client.post('/api/press/issue', body: {
        'template_id': templateId,
        'owner_pubkey': ownerPubkey,
      });

  Future<List<int>> downloadPdf(String serial) =>
      _client.getBytes('/api/press/pdf', params: {'serial': serial});
}
