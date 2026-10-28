import React from 'react'
import { ActivityIndicator, StyleSheet, Text, View } from 'react-native'
import { DarkTheme, NavigationContainer, type Theme } from '@react-navigation/native'
import { createNativeStackNavigator } from '@react-navigation/native-stack'

import { useSession } from '../context/ServicesProvider'
import { LoginScreen } from '../screens/LoginScreen'
import { OrdersScreen } from '../screens/OrdersScreen'
import { OrderDetailScreen } from '../screens/OrderDetailScreen'
import { RuntimeEffects } from '../components/RuntimeEffects'
import { colors, typography } from '../theme'
import type { RootStackParamList } from './types'

const Stack = createNativeStackNavigator<RootStackParamList>()

const navigationTheme: Theme = {
  ...DarkTheme,
  colors: {
    ...DarkTheme.colors,
    background: colors.background,
    card: colors.surface,
    text: colors.text,
    primary: colors.primary,
    border: colors.border,
    notification: colors.danger,
  },
}

/**
 * Decide la pantalla raíz según la sesión. Mientras no se hidrata la sesión no
 * se muestra el login para evitar un parpadeo con datos ya guardados.
 */
export function RootNavigator() {
  const snapshot = useSession()

  if (snapshot.status === 'unknown') {
    return (
      <View style={styles.splash}>
        <ActivityIndicator color={colors.primary} size="large" />
        <Text style={styles.splashText}>Restaurando sesión…</Text>
      </View>
    )
  }

  if (snapshot.status !== 'authenticated') {
    return <LoginScreen />
  }

  return (
    <>
      <RuntimeEffects />
      <NavigationContainer theme={navigationTheme}>
        <Stack.Navigator
          screenOptions={{
            headerStyle: { backgroundColor: colors.surface },
            headerTintColor: colors.text,
            headerTitleStyle: { fontWeight: '700' },
            contentStyle: { backgroundColor: colors.background },
            animation: 'slide_from_right',
          }}
        >
          <Stack.Screen name="Orders" component={OrdersScreen} options={{ headerShown: false }} />
          <Stack.Screen
            name="OrderDetail"
            component={OrderDetailScreen}
            options={{ title: 'Detalle de la entrega', headerBackTitle: 'Ruta' }}
          />
        </Stack.Navigator>
      </NavigationContainer>
    </>
  )
}

const styles = StyleSheet.create({
  splash: {
    flex: 1,
    backgroundColor: colors.background,
    alignItems: 'center',
    justifyContent: 'center',
    gap: 12,
  },
  splashText: { color: colors.muted, fontSize: typography.small },
})
