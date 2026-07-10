Regras especificas para Vue:
- Verifique props, emits, slots e v-model: contrato quebrado, evento nao emitido, prop obrigatoria ausente ou tipo incorreto.
- Verifique Composition API: ref/reactive, computed, watch/watchEffect, cleanup e dependencia reativa perdida.
- Verifique Options API: data, methods, computed, watchers e this usado de forma inconsistente.
- Verifique templates: v-if/v-for, key, binding dinamico, diretivas, filtros e acesso a valor possivelmente nulo.
- Verifique Pinia/Vuex, composables e services: estado compartilhado, side effects e tratamento de erro.
- Verifique Nuxt quando aparecer no diff: server/client boundary, route middleware, runtime config e fetch/cache.
- Nao reprove por estilo de single-file component, ordem de blocos ou preferencia entre Options/Composition API sem falha concreta.
