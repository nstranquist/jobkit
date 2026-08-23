<script setup lang="ts">
import DefaultTheme from 'vitepress/theme'
const { Layout } = DefaultTheme
</script>

<template>
  <Layout>
    <template #aside-outline-before>
      <p class="lab-chip">VitePress lab · v0.9.0</p>
    </template>
  </Layout>
</template>
