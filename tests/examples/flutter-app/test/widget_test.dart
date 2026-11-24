import 'package:flutter/material.dart';
import 'package:flutter_app/widgets/primary_button.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('PrimaryButton shows its label', (tester) async {
    await tester.pumpWidget(
      MaterialApp(home: PrimaryButton(label: 'Go', onPressed: () {})),
    );
    expect(find.text('Go'), findsOneWidget);
  });
}
