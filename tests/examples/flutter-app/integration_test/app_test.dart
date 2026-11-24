import 'package:flutter_app/app.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('starts with the login', (tester) async {
    await tester.pumpWidget(const App());
    expect(find.text('Sign in'), findsOneWidget);
  });
}
