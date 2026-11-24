import 'package:flutter/material.dart';

import '../../widgets/primary_button.dart';
import 'auth_service.dart';

class LoginScreen extends StatelessWidget {
  const LoginScreen({super.key});

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: Center(
        child: PrimaryButton(
          label: 'Sign in',
          onPressed: () async {
            await AuthService().signIn('demo@example.com', 'password');
            if (context.mounted) Navigator.of(context).pushReplacementNamed('/home');
          },
        ),
      ),
    );
  }
}
