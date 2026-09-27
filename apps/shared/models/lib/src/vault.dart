/// Vault models for file storage

/// VaultFile represents a file in the vault
class VaultFile {
  final String id;
  final String name;
  final int size;
  final String mimeType;
  final String hash;
  final String createdAt;

  VaultFile({
    required this.id,
    required this.name,
    required this.size,
    required this.mimeType,
    required this.hash,
    required this.createdAt,
  });

  factory VaultFile.fromJson(Map<String, dynamic> json) {
    return VaultFile(
      id: json['id'] as String? ?? '',
      name: json['name'] as String? ?? '',
      size: (json['size'] as num?)?.toInt() ?? 0,
      mimeType: json['mime_type'] as String? ?? '',
      hash: json['hash'] as String? ?? '',
      createdAt: json['created_at'] as String? ?? '',
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'id': id,
      'name': name,
      'size': size,
      'mime_type': mimeType,
      'hash': hash,
      'created_at': createdAt,
    };
  }
}

/// VaultListing represents a listing of vault files
class VaultListing {
  final List<VaultFile> files;

  VaultListing({required this.files});

  factory VaultListing.fromJson(Map<String, dynamic> json) {
    return VaultListing(
      files: (json['files'] as List<dynamic>?)
              ?.map((e) => VaultFile.fromJson(e as Map<String, dynamic>))
              .toList() ??
          [],
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'files': files.map((e) => e.toJson()).toList(),
    };
  }
}

/// UploadResult represents vault upload operation result
class UploadResult {
  final bool success;
  final String name;
  final int size;
  final String message;
  UploadResult({this.success = false, this.name = '', this.size = 0, this.message = ''});
  factory UploadResult.fromJson(Map<String, dynamic> json) => UploadResult(
    success: json['success'] ?? false,
    name: json['name'] ?? '',
    size: (json['size'] ?? 0).toInt(),
    message: json['message'] ?? '',
  );
}

/// DeleteResult represents vault delete operation result
class DeleteResult {
  final bool success;
  final String message;
  DeleteResult({this.success = false, this.message = ''});
  factory DeleteResult.fromJson(Map<String, dynamic> json) => DeleteResult(
    success: json['success'] ?? false,
    message: json['message'] ?? '',
  );
}