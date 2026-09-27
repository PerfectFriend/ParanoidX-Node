import 'dart:convert';
import 'dart:io';
import 'package:http/http.dart' as http;
import 'client.dart';

/// VaultClient manages vaultclient functionality.
class VaultClient {
  final SimplexNodeClient _client;
  VaultClient(this._client);

  Future<Map<String, dynamic>> list() =>
      _client.get('/api/vault/list');

  Future<Map<String, dynamic>> upload(String filePath, String fileName) async {
    final uri = Uri.parse('${_client.baseUrl}/api/vault/upload');
    final request = http.MultipartRequest('POST', uri);
    request.files.add(await http.MultipartFile.fromPath('file', filePath, filename: fileName));
    final streamed = await _client.httpClient.send(request);
    final response = await http.Response.fromStream(streamed);
    if (response.statusCode >= 400) {
      throw ApiException(response.statusCode, response.body);
    }
    return jsonDecode(response.body) as Map<String, dynamic>;
  }

  Future<List<int>> download(String name) async {
    final bytes = await _client.getBytes('/api/vault/download', params: {'name': name});
    return bytes.toList();
  }

  Future<Map<String, dynamic>> delete(String name) async {
    return _client.post('/api/vault/delete', params: {'name': name});
  }
}
