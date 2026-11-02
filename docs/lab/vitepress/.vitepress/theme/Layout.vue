<script setup lang="ts">
import DefaultTheme from 'vitepress/theme'
const { Layout } = DefaultTheme
</script>

<template>
  <Layout>
    <template #aside-outline-before>
      <p class="lab-chip">VitePress lab · v0.10.1</p>
    </template>
  </Layout>
</template>
