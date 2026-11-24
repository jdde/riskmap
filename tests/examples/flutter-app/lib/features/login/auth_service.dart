import 'dart:convert';

import 'package:http/http.dart' as http;

class AuthService {
  String? _token;

  bool get isSignedIn => _token != null;

  Future<void> signIn(String email, String password) async {
    final response = await http.post(
      Uri.parse('https://api.example.com/login'),
      body: {'email': email, 'password': password},
    );
    _token = (jsonDecode(response.body) as Map<String, dynamic>)['token'] as String?;
  }

  void signOut() => _token = null;
}
