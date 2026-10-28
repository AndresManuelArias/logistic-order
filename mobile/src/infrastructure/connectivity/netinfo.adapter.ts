import NetInfo, { type NetInfoState } from '@react-native-community/netinfo'

import type { ConnectivityListener, ConnectivityPort } from '../../ports/connectivity'

function isOnline(state: NetInfoState): boolean {
  // `isInternetReachable` es `null` mientras NetInfo comprueba; en ese caso se
  // confía en `isConnected` para no bloquear la sincronización al arrancar.
  return state.isConnected === true && state.isInternetReachable !== false
}

/** Adaptador de NetInfo: conectividad real, no sólo interfaz de red. */
export class NetInfoConnectivity implements ConnectivityPort {
  async isOnline(): Promise<boolean> {
    const state = await NetInfo.fetch()
    return isOnline(state)
  }

  subscribe(listener: ConnectivityListener): () => void {
    let last: boolean | null = null
    return NetInfo.addEventListener((state) => {
      const online = isOnline(state)
      if (online === last) return
      last = online
      listener(online)
    })
  }
}
