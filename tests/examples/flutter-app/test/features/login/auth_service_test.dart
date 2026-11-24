import 'package:flutter_app/features/login/auth_service.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('is signed out after signOut', () {
    final service = AuthService()..signOut();
    expect(service.isSignedIn, isFalse);
  });
}
