import 'dart:convert';
import 'dart:typed_data';
import 'package:http/http.dart' as http;

/// SimplexNodeClient manages simplexnodeclient functionality.
class SimplexNodeClient {
  String baseUrl;
  final http.Client _client;
  bool _useTransport;
  int _reqCounter = 0;

  SimplexNodeClient(this.baseUrl, {http.Client? client, bool useTransport = true})
      : _client = client ?? http.Client(),
        _useTransport = useTransport;

  set useTransport(bool v) => _useTransport = v;

  http.Client get httpClient => _client;

  Future<Map<String, dynamic>> get(String path, {Map<String, String>? params}) async {
    final fullPath = params != null && params.isNotEmpty
        ? '$path?${params.entries.map((e) => '${Uri.encodeComponent(e.key)}=${Uri.encodeComponent(e.value)}').join('&')}'
        : path;

    if (!_useTransport) {
      final uri = Uri.parse('$baseUrl$fullPath');
      final res = await _client.get(uri);
      _checkError(res);
      return jsonDecode(res.body);
    }

    return _transportCall('GET', fullPath, null);
  }

  Future<Map<String, dynamic>> post(String path, {Map<String, String>? params, Map<String, dynamic>? body}) async {
    final fullPath = params != null && params.isNotEmpty
        ? '$path?${params.entries.map((e) => '${Uri.encodeComponent(e.key)}=${Uri.encodeComponent(e.value)}').join('&')}'
        : path;

    if (!_useTransport) {
      final uri = Uri.parse('$baseUrl$fullPath');
      final res = await _client.post(
        uri,
        headers: {'Content-Type': 'application/json'},
        body: body != null ? jsonEncode(body) : null,
      );
      _checkError(res);
      return jsonDecode(res.body);
    }

    return _transportCall('POST', fullPath, body);
  }

  Future<Uint8List> getBytes(String path, {Map<String, String>? params}) async {
    final fullPath = params != null && params.isNotEmpty
        ? '$path?${params.entries.map((e) => '${Uri.encodeComponent(e.key)}=${Uri.encodeComponent(e.value)}').join('&')}'
        : path;

    if (!_useTransport) {
      final uri = Uri.parse('$baseUrl$fullPath');
      final res = await _client.get(uri);
      _checkError(res);
      return res.bodyBytes;
    }

    final result = await _transportCall('GET', fullPath, null);
    final bodyStr = result['body'] as String?;
    if (bodyStr != null) {
      return Uint8List.fromList(utf8.encode(bodyStr));
    }
    return Uint8List(0);
  }

  Future<Map<String, dynamic>> _transportCall(String method, String path, Map<String, dynamic>? body) async {
    _reqCounter++;
    final envelope = {
      'type': 'api.request',
      'payload': {
        'method': method,
        'path': path,
        'body': body,
      },
      'id': 'req-$_reqCounter',
    };

    final uri = Uri.parse('$baseUrl/api/transport/send');
    final res = await _client.post(
      uri,
      headers: {'Content-Type': 'application/json'},
      body: jsonEncode(envelope),
    );

    if (res.statusCode >= 400) {
      throw ApiException(res.statusCode, res.body);
    }

    final responseEnvelope = jsonDecode(res.body) as Map<String, dynamic>;
    if (responseEnvelope['type'] == 'error') {
      throw ApiException(502, responseEnvelope['payload']?.toString() ?? 'transport error');
    }

    final apiResp = responseEnvelope['payload'] as Map<String, dynamic>;
    final status = apiResp['status'] as int;
    final respBodyRaw = apiResp['body'];

    Map<String, dynamic> respBody;
    if (respBodyRaw is String) {
      respBody = jsonDecode(respBodyRaw) as Map<String, dynamic>;
    } else if (respBodyRaw is Map) {
      respBody = respBodyRaw as Map<String, dynamic>;
    } else {
      respBody = {};
    }

    if (status >= 400) {
      throw ApiException(status, jsonEncode(respBody));
    }

    return respBody;
  }

  void _checkError(http.Response res) {
    if (res.statusCode >= 400) {
      throw ApiException(res.statusCode, res.body);
    }
  }

  void dispose() => _client.close();
}

/// ApiException manages exception representing a non-200 HTTP response from the API.
class ApiException implements Exception {
  final int statusCode;
  final String body;
  ApiException(this.statusCode, this.body);

  @override
  String toString() => 'ApiException($statusCode): $body';
}
