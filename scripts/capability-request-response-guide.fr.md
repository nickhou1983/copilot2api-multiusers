# Tests de capacités Claude : guide d'apprentissage des requêtes et des réponses

> Généré le : 2026-07-16 15:52 ・ Modèle : `claude-sonnet-4.6` ・ Compte amont : enterprise
> Source des données : mesures réelles avec `scripts/capability_test.py --target both` (accès direct à l'amont GitHub Copilot + proxy copilot2api).
> Pour la lisibilité : le champ de détail de facturation Copilot `copilot_usage` est omis des réponses ; les données base64 et les chaînes très longues sont tronquées.
> Sauf mention contraire, les requêtes et réponses présentées ci-dessous sont celles du **proxy** (route native `/v1/messages`) ; lorsque le résultat en accès direct diffère, il est indiqué séparément.

## Sommaire

**Capacités de base**
- 1. [`text`](#1-text) Texte de base
- 2. [`streaming`](#2-streaming) Sortie en streaming

**Appel d'outils**
- 3. [`function_calling`](#3-function-calling) Appel de fonctions
- 4. [`parallel_tools`](#4-parallel-tools) Appels d'outils parallèles

**Multimodal**
- 5. [`vision_base64`](#5-vision-base64) Compréhension d'images (base64)
- 6. [`vision_url`](#6-vision-url) Compréhension d'images (URL)
- 7. [`pdf_document`](#7-pdf-document) Compréhension de documents PDF

**Réflexion / raisonnement**
- 8. [`extended_thinking`](#8-extended-thinking) Réflexion étendue

**Outils intégrés**
- 9. [`server_tool_bash`](#9-server-tool-bash) Outil Bash
- 10. [`server_tool_text_editor`](#10-server-tool-text-editor) Outil éditeur de texte
- 11. [`server_tool_memory`](#11-server-tool-memory) Outil de mémoire

**Cache / contexte**
- 12. [`prompt_cache`](#12-prompt-cache) Cache de prompt (point d'arrêt au niveau du bloc)
- 13. [`cache_control_scope`](#13-cache-control-scope) Champ scope du cache
- 14. [`context_management`](#14-context-management) Édition du contexte (purge des résultats d'outils)

**RAG / sortie**
- 15. [`citations`](#15-citations) Citations de documents

**Outils côté serveur**
- 16. [`web_search`](#16-web-search) Recherche web

**Outils intégrés**
- 17. [`computer_use`](#17-computer-use) Contrôle de l'ordinateur

**Capacités de base**
- 18. [`count_tokens`](#18-count-tokens) Comptage de tokens

**Cache / contexte**
- 19. [`context_1m`](#19-context-1m) Contexte de 1M

**Paramètres d'échantillonnage**
- 20. [`temperature`](#20-temperature) Échantillonnage par température
- 21. [`top_p`](#21-top-p) Échantillonnage nucléus
- 22. [`top_k`](#22-top-k) Échantillonnage Top-K
- 23. [`stop_sequences`](#23-stop-sequences) Séquences d'arrêt personnalisées
- 24. [`metadata`](#24-metadata) Métadonnées de requête
- 25. [`service_tier`](#25-service-tier) Niveau de service

**Appel d'outils**
- 26. [`tool_choice_auto`](#26-tool-choice-auto) Choix d'outil : automatique
- 27. [`tool_choice_any`](#27-tool-choice-any) Choix d'outil : forcer n'importe lequel
- 28. [`tool_choice_tool`](#28-tool-choice-tool) Choix d'outil : forcer un outil précis
- 29. [`tool_choice_none`](#29-tool-choice-none) Choix d'outil : désactivé
- 30. [`tool_choice_no_parallel`](#30-tool-choice-no-parallel) Désactivation des outils parallèles

**RAG / sortie**
- 31. [`structured_outputs`](#31-structured-outputs) Sorties structurées (JSON Schema)

**Outils côté serveur**
- 32. [`web_fetch`](#32-web-fetch) Récupération de pages web
- 33. [`code_execution`](#33-code-execution) Exécution de code
- 34. [`code_execution_beta_header`](#34-code-execution-beta-header) Exécution de code (avec en-tête beta)

**RAG / sortie**
- 35. [`search_result`](#35-search-result) Blocs de résultats de recherche

**Réflexion / raisonnement**
- 36. [`interleaved_thinking`](#36-interleaved-thinking) Réflexion entrelacée

**Appel d'outils**
- 37. [`token_efficient_tools`](#37-token-efficient-tools) Outils économes en tokens
- 38. [`fine_grained_tool_streaming`](#38-fine-grained-tool-streaming) Streaming d'outils à granularité fine

**Cache / contexte**
- 39. [`extended_cache_ttl`](#39-extended-cache-ttl) Cache d'une heure

**Réflexion / raisonnement**
- 40. [`effort_xhigh`](#40-effort-xhigh) effort=xhigh

**RAG / sortie**
- 41. [`output_300k`](#41-output-300k) Plafond de sortie de 300k

**Réflexion / raisonnement**
- 42. [`effort_max`](#42-effort-max) effort=max
- 43. [`thinking_budget`](#43-thinking-budget) Budget de réflexion manuel

**Plateforme / points de terminaison**
- 44. [`mid_conv_system`](#44-mid-conv-system) Message system en cours de conversation
- 45. [`fast_mode`](#45-fast-mode) Mode rapide

**Cache / contexte**
- 46. [`prompt_cache_1024`](#46-prompt-cache-1024) Seuil minimal de cache de 1024 tokens

**Plateforme / points de terminaison**
- 47. [`refusal_stop_details`](#47-refusal-stop-details) Détails de refus

**Capacités de base**
- 48. [`model_discovery`](#48-model-discovery) Découverte des modèles

**Cache / contexte**
- 49. [`auto_prompt_cache`](#49-auto-prompt-cache) Cache de prompt automatique

**Appel d'outils**
- 50. [`strict_tool_use`](#50-strict-tool-use) Appel d'outils strict

**Plateforme / points de terminaison**
- 51. [`inference_geo`](#51-inference-geo) Résidence des données

**Infrastructure d'outils**
- 52. [`mcp_connector`](#52-mcp-connector) Connecteur MCP
- 53. [`tool_search`](#53-tool-search) Recherche d'outils
- 54. [`programmatic_tool_calling`](#54-programmatic-tool-calling) Appel d'outils programmatique
- 55. [`agent_skills`](#55-agent-skills) Compétences d'agent

**Outils côté serveur**
- 56. [`advisor_tool`](#56-advisor-tool) Outil conseiller

**Cache / contexte**
- 57. [`compaction`](#57-compaction) Compactage côté serveur

**Plateforme / points de terminaison**
- 58. [`server_side_fallback`](#58-server-side-fallback) Repli côté serveur
- 59. [`batches_endpoint`](#59-batches-endpoint) API de traitement par lots
- 60. [`files_endpoint`](#60-files-endpoint) Files API


---

# Capacités de base

## 1. text

**Texte de base** (attendu : pris en charge)

L'échange textuel le plus simple, qui valide la chaîne requête → réponse. Le corps de la réponse est le bloc `text` du tableau `content` ; `usage` rapporte les tokens d'entrée/sortie.

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "messages": [
    {
      "role": "user",
      "content": "Reply with exactly: pong"
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "text": "pong",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01DrWJ5LUL1eN1SYNzMyTmKf",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 13,
    "output_tokens": 5
  }
}
```

## 2. streaming

**Sortie en streaming** (attendu : pris en charge)

Avec `stream: true`, la réponse est renvoyée fragment par fragment sous forme de flux d'événements SSE : `message_start` → `content_block_start` → plusieurs `content_block_delta` → `content_block_stop` → `message_delta` (avec stop_reason/usage) → `message_stop`.

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "stream": true,
  "messages": [
    {
      "role": "user",
      "content": "Count from 1 to 5."
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
Réponse SSE en streaming, 8 événements au total, types d'événements : `content_block_delta`, `content_block_start`, `content_block_stop`, `message_delta`, `message_start`, `message_stop`. Premiers événements :
```json
[
  {
    "message": {
      "content": [],
      "id": "msg_bdrk_01DABCPsGX5TwDweJ5apqymy",
      "model": "claude-sonnet-4-6",
      "role": "assistant",
      "stop_details": null,
      "stop_reason": null,
      "stop_sequence": null,
      "type": "message",
      "usage": {
        "cache_creation": {
          "ephemeral_1h_input_tokens": 0,
          "ephemeral_5m_input_tokens": 0
        },
        "cache_creation_input_tokens": 0,
        "cache_read_input_tokens": 0,
        "input_tokens": 16,
        "output_tokens": 3
      }
    },
    "type": "message_start"
  },
  {
    "content_block": {
      "text": "",
      "type": "text"
    },
    "index": 0,
    "type": "content_block_start"
  },
  {
    "delta": {
      "text": "1,",
      "type": "text_delta"
    },
    "index": 0,
    "type": "content_block_delta"
  },
  {
    "delta": {
      "text": " 2, 3, 4",
      "type": "text_delta"
    },
    "index": 0,
    "type": "content_block_delta"
  },
  {
    "delta": {
      "text": ", 5",
      "type": "text_delta"
    },
    "index": 0,
    "type": "content_block_delta"
  },
  {
    "index": 0,
    "type": "content_block_stop"
  },
  {
    "copilot_usage": {
      "token_details": [
        {
          "batch_size": 1000000,
          "cost_per_batch": 300000000000,
          "token_count": 16,
          "token_type": "input"
        },
        {
          "batch_size": 1000000,
          "cost_per_batch": 30000000000,
          "token_count": 0,
          "token_type": "cache_read"
        },
        {
          "batch_size": 1000000,
          "cost_per_batch": 375000000000,
          "token_count": 0,
          "token_type": "cache_write"
        },
        {
          "batch_size": 1000000,
          "cost_per_batch": 1500000000000,
          "token_count": 17,
          "token_type": "output"
        }
      ],
      "total_nano_aiu": 30300000
    },
    "delta": {
      "stop_details": null,
      "stop_reason": "end_turn",
      "stop_sequence": null
    },
    "type": "message_delta",
    "usage": {
      "cache_creation_input_tokens": 0,
      "cache_read_input_tokens": 0,
      "input_tokens": 16,
      "output_tokens": 17
    }
  },
  {
    "amazon-bedrock-invocationMetrics": {
      "firstByteLatency": 4016,
      "inputTokenCount": 16,
      "invocationLatency": 4164,
      "outputTokenCount": 17
    },
    "type": "message_stop"
  }
]
```

---

# Appel d'outils

## 3. function_calling

**Appel de fonctions** (attendu : pris en charge)

On définit les fonctions dans `tools` (nom + JSON Schema des paramètres) ; le modèle renvoie un bloc de contenu `tool_use` (avec `id`/`name`/`input`) et `stop_reason` vaut `tool_use`. Après exécution, le client renvoie un bloc `tool_result` pour poursuivre la conversation.

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "name": "get_weather",
      "description": "Get current weather for a city",
      "input_schema": {
        "type": "object",
        "properties": {
          "city": {
            "type": "string"
          }
        },
        "required": [
          "city"
        ]
      }
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "What's the weather in Paris? Use the tool."
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "id": "toolu_bdrk_01Gduoj67Uh645TQjY2g5sFT",
      "input": {
        "city": "Paris"
      },
      "name": "get_weather",
      "type": "tool_use"
    }
  ],
  "id": "msg_bdrk_01WFhmzE3yx84bGVw6DF3v7w",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "tool_use",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 569,
    "output_tokens": 54
  }
}
```

## 4. parallel_tools

**Appels d'outils parallèles** (attendu : pris en charge)

Plusieurs blocs `tool_use` sont renvoyés dans une même réponse afin d'appeler plusieurs outils en parallèle, ce qui réduit le nombre d'allers-retours.

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "name": "get_weather",
      "description": "Get current weather for a city",
      "input_schema": {
        "type": "object",
        "properties": {
          "city": {
            "type": "string"
          }
        },
        "required": [
          "city"
        ]
      }
    },
    {
      "name": "get_time",
      "description": "Get current time for a timezone",
      "input_schema": {
        "type": "object",
        "properties": {
          "tz": {
            "type": "string"
          }
        },
        "required": [
          "tz"
        ]
      }
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Get BOTH the weather in Paris and the time in Asia/Tokyo. Call both tools."
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "text": "Sure! I'll fetch both the weather in Paris and the current time in Asia/Tokyo simultaneously right away!",
      "type": "text"
    },
    {
      "id": "toolu_bdrk_013h4SrVKD44Nfs3xDHYV6Ca",
      "input": {
        "city": "Paris"
      },
      "name": "get_weather",
      "type": "tool_use"
    },
    {
      "id": "toolu_bdrk_01DANp4TLnMo83GTt3c1byyx",
      "input": {
        "tz": "Asia/Tokyo"
      },
      "name": "get_time",
      "type": "tool_use"
    }
  ],
  "id": "msg_bdrk_01E8yBVUL8btpCcrsyfJxFoF",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "tool_use",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 633,
    "output_tokens": 115
  }
}
```

---

# Multimodal

## 5. vision_base64

**Compréhension d'images (base64)** (attendu : pris en charge)

Le bloc de contenu `image` embarque les données de l'image en base64 (`source.type: "base64"` + `media_type` + `data`) ; le modèle décrit ou analyse le contenu de l'image.

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "messages": [
    {
      "role": "user",
      "content": [
        {
          "type": "image",
          "source": {
            "type": "base64",
            "media_type": "image/png",
            "data": "iVBORw0KGgoAAAANSUhEUgAAAMgAAADICAIAAAAiOjnJAAABeElEQVR42u3S...(base64, 414 caractères au total, tronqué)"
          }
        },
        {
          "type": "text",
          "text": "What color dominates this image? One word."
        }
      ]
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "text": "**Crimson**",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01Kgqyz3E9e47urXPoVRjbsv",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 85,
    "output_tokens": 8
  }
}
```

## 6. vision_url

**Compréhension d'images (URL)** (attendu : rejeté / absent)

`source.type: "url"` référence l'URL d'une image externe. L'amont Copilot ne prend pas en charge les images externes → rejet 400 (sonde figeant le comportement).

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "messages": [
    {
      "role": "user",
      "content": [
        {
          "type": "image",
          "source": {
            "type": "url",
            "url": "https://example.com/x.png"
          }
        },
        {
          "type": "text",
          "text": "Describe."
        }
      ]
    }
  ]
}
```

**Réponse** (proxy, HTTP 400 ✅)
```json
{
  "error": {
    "message": "external image URLs are not supported",
    "code": ""
  }
}
```

## 7. pdf_document

**Compréhension de documents PDF** (attendu : pris en charge)

Le bloc de contenu `document` embarque un PDF en base64 (`media_type: "application/pdf"`) ; le modèle analyse le texte ainsi que le contenu visuel de la mise en page.

**Requête** `POST /v1/messages` (anthropic-beta : pdfs-2024-09-25)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "messages": [
    {
      "role": "user",
      "content": [
        {
          "type": "document",
          "source": {
            "type": "base64",
            "media_type": "application/pdf",
            "data": "JVBERi0xLjQKMSAwIG9iago8PCAvVHlwZSAvQ2F0YWxvZyAvUGFnZXMgMiAw...(base64, 414 caractères au total, tronqué)"
          }
        },
        {
          "type": "text",
          "text": "What secret marker appears in this PDF?"
        }
      ]
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "text": "The secret marker in the PDF is **BANANA**.",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_014P99GzU61R3wxbeBzKUky7",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 1606,
    "output_tokens": 14
  }
}
```

---

# Réflexion / raisonnement

## 8. extended_thinking

**Réflexion étendue** (attendu : pris en charge)

Le paramètre `thinking` rend le raisonnement explicite : la réponse produit d'abord un bloc `thinking` (raisonnement pas à pas visible), puis un bloc `text`. Deux formes existent, budget manuel (`type:"enabled"+budget_tokens`) et adaptative (`type:"adaptive"`) ; le cas de test embarque les deux variantes pour couvrir les différents modèles.

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 2048,
  "thinking": {
    "type": "enabled",
    "budget_tokens": 1024
  },
  "messages": [
    {
      "role": "user",
      "content": "Think briefly, then answer: what is 17 * 23?"
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "signature": "EswBCmcIDxABGAIqQGO/A+F/j0tzmWTO/uWXC1SOSU9svZiAehnCxpMWzbOyDGJC+cw8F4Jh7+s3fb6aW0QQQiHmy75OIFn51NC5pOYyEWNsYXVkZS1zb25uZXQtNC02OABCCHRoaW5raW5nEgwatOQFNUMOG8MOGwQaDP3Fp8MKrOut69+8ESIwOwhlOKcEv/BqTOcRfkc5arqPNHIfP5IEh4zzlufybVp5tbxLGlzB87mY+6jQKYukKhMU7eAK5Wf8T4twQ2wpMw/NYQcKGAE=",
      "thinking": "391",
      "type": "thinking"
    },
    {
      "text": "**17 × 23 = 391**\n\nQuick breakdown: 17 × 20 = 340, plus 17 × 3 = 51 → 340 + 51 = **391**",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01NQEXWdJNdLKZSx6fArRCSX",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 51,
    "output_tokens": 62,
    "output_tokens_details": {
      "thinking_tokens": 7
    }
  }
}
```

---

# Outils intégrés

## 9. server_tool_bash

**Outil Bash** (attendu : pris en charge)

Type d'outil intégré `bash_20250124` : le modèle demande l'exécution de commandes shell (il renvoie `tool_use`, l'exécution réelle revenant au client). C'est la base d'agents comme Claude Code.

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "type": "bash_20250124",
      "name": "bash"
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Use the bash tool to help with a trivial task, or just acknowledge."
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "id": "toolu_bdrk_01UcfP4eiC7QYyyqkVwFZNqX",
      "input": {
        "command": "echo \"Ready to help!\""
      },
      "name": "bash",
      "type": "tool_use"
    }
  ],
  "id": "msg_bdrk_01SVTZ7H24T8BRK36eMTW6jY",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "tool_use",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 764,
    "output_tokens": 57
  }
}
```

## 10. server_tool_text_editor

**Outil éditeur de texte** (attendu : pris en charge)

Outil intégré `text_editor_20250728` (`str_replace_based_edit_tool`) : interface standard pour consulter, créer et modifier des fichiers.

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "type": "text_editor_20250728",
      "name": "str_replace_based_edit_tool"
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Use the text_editor tool to help with a trivial task, or just acknowledge."
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "text": "Sure! I'm ready to help with any text editing tasks. The `str_replace_based_edit_tool` is available and can:\n\n- **`view`** – View files or directory structures\n- **`create`** – Create or overwrite files\n- **`str_replace`** – Replace specific text in a file\n- **`insert`** – Insert text at a specific line\n\nJust let me know what you'd like to do — whether it's creating a new file, editing an existing...(truncated)",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01BwvsWCmobmfkvXyp64HvqS",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 1267,
    "output_tokens": 126
  }
}
```

## 11. server_tool_memory

**Outil de mémoire** (attendu : pris en charge)

Outil intégré `memory_20250818` : lecture et écriture de mémoire d'une session à l'autre, pour construire une base de connaissances durable.

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "type": "memory_20250818",
      "name": "memory"
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Use the memory tool to help with a trivial task, or just acknowledge."
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "id": "toolu_bdrk_011Z2Ez6vrewBsNMehaSnePs",
      "input": {
        "command": "view",
        "path": "/memories"
      },
      "name": "memory",
      "type": "tool_use"
    }
  ],
  "id": "msg_bdrk_01BxTfJEPH2YWm6PFr8WBtkm",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "tool_use",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 1589,
    "output_tokens": 70
  }
}
```

---

# Cache / contexte

## 12. prompt_cache

**Cache de prompt (point d'arrêt au niveau du bloc)** (attendu : pris en charge)

Ajouter `cache_control: {type:"ephemeral"}` sur un bloc de contenu définit un point d'arrêt de cache. À la première requête, `cache_creation_input_tokens` > 0 (écriture du cache, 1,25x) ; ensuite les correspondances apparaissent dans `cache_read_input_tokens` (0,1x). La réponse de cette exécution affiche `cr: 1202`, signe que le cache précédent a été touché.

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "system": [
    {
      "type": "text",
      "text": "You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assis...(truncated)",
      "cache_control": {
        "type": "ephemeral"
      }
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Say hi."
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "text": "Hi! 👋 How can I help you today?",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01CZxZFWtwLL7jZs3pyLGNER",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 1202
    },
    "cache_creation_input_tokens": 1202,
    "cache_read_input_tokens": 0,
    "input_tokens": 9,
    "output_tokens": 16
  }
}
```

## 13. cache_control_scope

**Champ scope du cache** (attendu : pris en charge)

Claude Code envoie `cache_control.scope: "global"` (qui contrôle le partage du cache entre régions). L'amont ne connaît pas ce champ et répond 400 ; ce cas vérifie que le proxy retire correctement scope tout en conservant un cache opérationnel.

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "system": [
    {
      "type": "text",
      "text": "You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assis...(truncated)",
      "cache_control": {
        "type": "ephemeral",
        "scope": "global"
      }
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Say hi."
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "text": "Hi! 👋 How are you doing? Is there something I can help you with today?",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01HRTibx8atHdjn5S45a7UgM",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 1202,
    "input_tokens": 9,
    "output_tokens": 24
  }
}
```

**Différence en accès direct à l'amont** (HTTP 400)
```json
{
  "type": "error",
  "error": {
    "type": "invalid_request_error",
    "message": "system.0.cache_control.ephemeral.scope: Extra inputs are not permitted"
  },
  "request_id": "req_011Cd5MhDWU95csxMb5nghzy"
}
```

## 14. context_management

**Édition du contexte (purge des résultats d'outils)** (attendu : pris en charge)

Stratégie `clear_tool_uses_20250919` de `context_management.edits` : au-delà du seuil trigger, le serveur supprime automatiquement les résultats d'outils les plus anciens. Le champ `context_management.applied_edits` de la réponse rapporte la quantité réellement purgée. Le proxy ajoute automatiquement l'en-tête beta `context-management-2025-06-27`.

**Requête** `POST /v1/messages` (anthropic-beta : context-management-2025-06-27)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 256,
  "tools": [
    {
      "name": "lookup",
      "description": "Look up a record",
      "input_schema": {
        "type": "object",
        "properties": {
          "id": {
            "type": "string"
          }
        },
        "required": [
          "id"
        ]
      }
    }
  ],
  "context_management": {
    "edits": [
      {
        "type": "clear_tool_uses_20250919",
        "trigger": {
          "type": "input_tokens",
          "value": 1
        },
        "keep": {
          "type": "tool_uses",
          "value": 0
        }
      }
    ]
  },
  "messages": [
    {
      "role": "user",
      "content": "Look up record 42."
    },
    {
      "role": "assistant",
      "content": [
        {
          "type": "tool_use",
          "id": "toolu_cm1",
          "name": "lookup",
          "input": {
            "id": "42"
          }
        }
      ]
    },
    {
      "role": "user",
      "content": [
        {
          "type": "tool_result",
          "tool_use_id": "toolu_cm1",
          "content": "RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECO...(truncated)"
        }
      ]
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "text": "The lookup for record **42** has been completed, but the result was cleared or returned empty. This could mean:\n\n- The record **42** does not exist in the system.\n- The data was intentionally cleared or is unavailable.\n\nWould you like me to try again, or is there another record you'd like to look up?",
      "type": "text"
    }
  ],
  "context_management": {
    "applied_edits": [
      {
        "cleared_input_tokens": 1214,
        "cleared_tool_uses": 1,
        "type": "clear_tool_uses_20250919"
      }
    ]
  },
  "id": "msg_bdrk_014Uuo8PBq9SKRRHegZacd11",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 632,
    "output_tokens": 75
  }
}
```

---

# RAG / sortie

## 15. citations

**Citations de documents** (attendu : pris en charge)

Une fois `citations: {enabled: true}` activé sur un bloc `document`, les blocs `text` de la réponse portent un tableau `citations` indiquant la provenance au niveau de la phrase (char_location).

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "messages": [
    {
      "role": "user",
      "content": [
        {
          "type": "document",
          "source": {
            "type": "base64",
            "media_type": "application/pdf",
            "data": "JVBERi0xLjQKMSAwIG9iago8PCAvVHlwZSAvQ2F0YWxvZyAvUGFnZXMgMiAw...(base64, 414 caractères au total, tronqué)"
          },
          "citations": {
            "enabled": true
          }
        },
        {
          "type": "text",
          "text": "What is the secret marker? Cite the document."
        }
      ]
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "citations": [
        {
          "cited_text": "The secret marker is BANANA",
          "document_index": 0,
          "document_title": null,
          "end_page_number": 2,
          "start_page_number": 1,
          "type": "page_location"
        }
      ],
      "text": "The secret marker is **BANANA**.",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01JhFZED5pKws9KrtZVyLugv",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 2125,
    "output_tokens": 28
  }
}
```

---

# Outils côté serveur

## 16. web_search

**Recherche web** (attendu : rejeté / absent)

Outil de recherche côté serveur `web_search_20250305`. L'amont Copilot le refuse explicitement (400 "not supported") — sonde de rejet.

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "type": "web_search_20250305",
      "name": "web_search"
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Search the web for today's news."
    }
  ]
}
```

**Réponse** (proxy, HTTP 400 ✅)
```json
{
  "error": {
    "message": "The use of the web search tool is not supported.",
    "code": "unsupported_value"
  }
}
```

---

# Outils intégrés

## 17. computer_use

**Contrôle de l'ordinateur** (attendu : pris en charge)

Outil `computer_20251124` + en-tête beta `computer-use-2025-11-24` : capture d'écran et pilotage de l'interface graphique au clavier et à la souris. Le proxy retransmet l'en-tête beta computer-use du client (c'est la lacune corrigée dans une version précédente).

**Requête** `POST /v1/messages` (anthropic-beta : computer-use-2025-11-24)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "type": "computer_20251124",
      "name": "computer",
      "display_width_px": 1024,
      "display_height_px": 768
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Take a screenshot."
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "text": "I'll take a screenshot right away!",
      "type": "text"
    },
    {
      "id": "toolu_bdrk_01VMygiqyXYb9zhQMyQNfjEY",
      "input": {
        "action": "screenshot"
      },
      "name": "computer",
      "type": "tool_use"
    }
  ],
  "id": "msg_bdrk_01K4P1WertqsydMULuf4fD5F",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "tool_use",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 1833,
    "output_tokens": 60
  }
}
```

---

# Capacités de base

## 18. count_tokens

**Comptage de tokens** (attendu : pris en charge)

`POST /v1/messages/count_tokens` : estime le nombre de tokens d'un message avant envoi ; la réponse ne contient que `{"input_tokens": N}`.

**Requête** `POST /v1/messages/count_tokens` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "messages": [
    {
      "role": "user",
      "content": "How many tokens is this sentence?"
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "input_tokens": 14
}
```

---

# Cache / contexte

## 19. context_1m

**Contexte de 1M** (attendu : pris en charge)

L'en-tête beta `context-1m-2025-08-07` demande une fenêtre de contexte de 1M tokens. Sur les nouveaux modèles (Sonnet 4.6+), le modèle de base offre déjà 1M et le proxy n'ajoute plus aveuglément le suffixe `-1m`.

**Requête** `POST /v1/messages` (anthropic-beta : context-1m-2025-08-07)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "messages": [
    {
      "role": "user",
      "content": "Reply with exactly: pong"
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "text": "pong",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01WXaykySPRDgTpPMnpAtkQM",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 13,
    "output_tokens": 5
  }
}
```

---

# Paramètres d'échantillonnage

## 20. temperature

**Échantillonnage par température** (attendu : pris en charge)

`temperature` contrôle le caractère aléatoire de la sortie (0 = déterministe, 1 = aléa maximal). Vérifie le transfert champ par champ du passthrough natif.

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "temperature": 0.0,
  "messages": [
    {
      "role": "user",
      "content": "Reply with exactly: pong"
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "text": "pong",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01VSdL4s2e6sz6eAoUQfA26H",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 13,
    "output_tokens": 5
  }
}
```

## 21. top_p

**Échantillonnage nucléus** (attendu : pris en charge)

`top_p` tronque la liste des candidats selon la probabilité cumulée (à régler en alternative à temperature).

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "top_p": 0.5,
  "messages": [
    {
      "role": "user",
      "content": "Reply with exactly: pong"
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "text": "pong",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01QRstSnoBnhWhHomhVLE31M",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 13,
    "output_tokens": 5
  }
}
```

## 22. top_k

**Échantillonnage Top-K** (attendu : pris en charge)

`top_k` n'échantillonne que parmi les K candidats les plus probables.

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "top_k": 10,
  "messages": [
    {
      "role": "user",
      "content": "Reply with exactly: pong"
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "text": "pong",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01R9AukHdWvPMLGU39iZoZep",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 13,
    "output_tokens": 5
  }
}
```

## 23. stop_sequences

**Séquences d'arrêt personnalisées** (attendu : pris en charge)

Tableau `stop_sequences` : le modèle s'arrête dès qu'il produit l'une des chaînes indiquées ; `stop_reason` vaut alors `stop_sequence` et le champ `stop_sequence` indique laquelle a été rencontrée.

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "stop_sequences": [
    "STOP"
  ],
  "messages": [
    {
      "role": "user",
      "content": "Repeat this text verbatim and nothing else: alpha bravo STOP charlie delta"
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "text": "alpha bravo ",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01XC2aYsU1JzZQAMVp8MxVms",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "stop_sequence",
  "stop_sequence": "STOP",
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 25,
    "output_tokens": 5
  }
}
```

## 24. metadata

**Métadonnées de requête** (attendu : pris en charge)

`metadata.user_id` étiquette la requête (traçage des abus, audit) sans influencer la génération.

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "metadata": {
    "user_id": "capability-test-user"
  },
  "messages": [
    {
      "role": "user",
      "content": "Reply with exactly: pong"
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "text": "pong",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01CxBFm6oUCg1einisnFkw6d",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 13,
    "output_tokens": 5
  }
}
```

## 25. service_tier

**Niveau de service** (attendu : pris en charge)

`service_tier` (auto/standard_only) sélectionne le palier de capacité prioritaire.

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "service_tier": "auto",
  "messages": [
    {
      "role": "user",
      "content": "Reply with exactly: pong"
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "text": "pong",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01B4aa8XGWQJqodnauWd8szr",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 13,
    "output_tokens": 5
  }
}
```

---

# Appel d'outils

## 26. tool_choice_auto

**Choix d'outil : automatique** (attendu : pris en charge)

`tool_choice: {type:"auto"}` (par défaut) : le modèle décide lui-même s'il appelle un outil.

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "name": "get_weather",
      "description": "Get current weather for a city",
      "input_schema": {
        "type": "object",
        "properties": {
          "city": {
            "type": "string"
          }
        },
        "required": [
          "city"
        ]
      }
    }
  ],
  "tool_choice": {
    "type": "auto"
  },
  "messages": [
    {
      "role": "user",
      "content": "What's the weather in Paris?"
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "id": "toolu_bdrk_01LRKSGXyRCkZAiVpMNxokUK",
      "input": {
        "city": "Paris"
      },
      "name": "get_weather",
      "type": "tool_use"
    }
  ],
  "id": "msg_bdrk_01Jvbt9yPLHbEGEQ5sCGSDTj",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "tool_use",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 565,
    "output_tokens": 54
  }
}
```

## 27. tool_choice_any

**Choix d'outil : forcer n'importe lequel** (attendu : pris en charge)

`tool_choice: {type:"any"}` : au moins un outil doit être appelé, une réponse purement textuelle est interdite.

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "name": "get_weather",
      "description": "Get current weather for a city",
      "input_schema": {
        "type": "object",
        "properties": {
          "city": {
            "type": "string"
          }
        },
        "required": [
          "city"
        ]
      }
    },
    {
      "name": "get_time",
      "description": "Get current time for a timezone",
      "input_schema": {
        "type": "object",
        "properties": {
          "tz": {
            "type": "string"
          }
        },
        "required": [
          "tz"
        ]
      }
    }
  ],
  "tool_choice": {
    "type": "any"
  },
  "messages": [
    {
      "role": "user",
      "content": "Help me plan a trip to Paris."
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "id": "toolu_bdrk_019txnbU5MJG5CPoVpnzDXMg",
      "input": {
        "city": "Paris"
      },
      "name": "get_weather",
      "type": "tool_use"
    },
    {
      "id": "toolu_bdrk_01G2BPxkXY4v7CkvsSXyteGS",
      "input": {
        "tz": "Europe/Paris"
      },
      "name": "get_time",
      "type": "tool_use"
    }
  ],
  "id": "msg_bdrk_01RqzQVKfeCe4RxtcB5FncMf",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "tool_use",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 714,
    "output_tokens": 77
  }
}
```

## 28. tool_choice_tool

**Choix d'outil : forcer un outil précis** (attendu : pris en charge)

`tool_choice: {type:"tool", name:"get_weather"}` : force l'appel de l'outil portant le nom indiqué.

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "name": "get_weather",
      "description": "Get current weather for a city",
      "input_schema": {
        "type": "object",
        "properties": {
          "city": {
            "type": "string"
          }
        },
        "required": [
          "city"
        ]
      }
    },
    {
      "name": "get_time",
      "description": "Get current time for a timezone",
      "input_schema": {
        "type": "object",
        "properties": {
          "tz": {
            "type": "string"
          }
        },
        "required": [
          "tz"
        ]
      }
    }
  ],
  "tool_choice": {
    "type": "tool",
    "name": "get_weather"
  },
  "messages": [
    {
      "role": "user",
      "content": "Do something useful for Paris."
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "id": "toolu_bdrk_017LYLVTtRNc1pTGG65SY4bh",
      "input": {
        "city": "Paris"
      },
      "name": "get_weather",
      "type": "tool_use"
    },
    {
      "id": "toolu_bdrk_01BCpnfRAJtPq7Ky5o1tbiMM",
      "input": {
        "tz": "Europe/Paris"
      },
      "name": "get_time",
      "type": "tool_use"
    }
  ],
  "id": "msg_bdrk_01Quvd9FNeZZpiPr87JoJSTQ",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "tool_use",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 717,
    "output_tokens": 72
  }
}
```

## 29. tool_choice_none

**Choix d'outil : désactivé** (attendu : pris en charge)

`tool_choice: {type:"none"}` : interdit l'appel d'outils, seule une réponse textuelle est possible (`stop_reason: end_turn`).

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "name": "get_weather",
      "description": "Get current weather for a city",
      "input_schema": {
        "type": "object",
        "properties": {
          "city": {
            "type": "string"
          }
        },
        "required": [
          "city"
        ]
      }
    }
  ],
  "tool_choice": {
    "type": "none"
  },
  "messages": [
    {
      "role": "user",
      "content": "What's the weather in Paris? Answer in plain text, do not call any tool."
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "text": "I'm sorry, but I don't have the ability to provide current weather information without using tools. My built-in knowledge doesn't include real-time data like live weather updates. I'd recommend checking a weather website or app (such as weather.com or a local forecast service) for the most accurate and up-to-date weather in Paris.",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01USLh8HxN2JyMLBHMhJnwbJ",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 576,
    "output_tokens": 75
  }
}
```

## 30. tool_choice_no_parallel

**Désactivation des outils parallèles** (attendu : pris en charge)

`disable_parallel_tool_use: true` : au plus un bloc `tool_use` par tour, adapté aux opérations sensibles à l'ordre.

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "name": "get_weather",
      "description": "Get current weather for a city",
      "input_schema": {
        "type": "object",
        "properties": {
          "city": {
            "type": "string"
          }
        },
        "required": [
          "city"
        ]
      }
    },
    {
      "name": "get_time",
      "description": "Get current time for a timezone",
      "input_schema": {
        "type": "object",
        "properties": {
          "tz": {
            "type": "string"
          }
        },
        "required": [
          "tz"
        ]
      }
    }
  ],
  "tool_choice": {
    "type": "any",
    "disable_parallel_tool_use": true
  },
  "messages": [
    {
      "role": "user",
      "content": "Get BOTH the weather in Paris and the time in Asia/Tokyo."
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "id": "toolu_bdrk_012o2jsFAeGrKWtkff3E9vax",
      "input": {
        "city": "Paris"
      },
      "name": "get_weather",
      "type": "tool_use"
    }
  ],
  "id": "msg_bdrk_01WWhLHdVEdUAhPEhLS2ZnWs",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "tool_use",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 722,
    "output_tokens": 29
  }
}
```

---

# RAG / sortie

## 31. structured_outputs

**Sorties structurées (JSON Schema)** (attendu : pris en charge)

`output_config.format` fournit un JSON Schema ; le décodage contraint garantit que la réponse est un JSON valide et conforme au schéma — sans réessai de parsing.

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "messages": [
    {
      "role": "user",
      "content": "Extract structured data: it is 22 degrees Celsius in Paris right now."
    }
  ],
  "output_config": {
    "format": {
      "type": "json_schema",
      "schema": {
        "type": "object",
        "properties": {
          "city": {
            "type": "string"
          },
          "temp_c": {
            "type": "number"
          }
        },
        "required": [
          "city",
          "temp_c"
        ],
        "additionalProperties": false
      }
    }
  }
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "text": "{\"city\":\"Paris\",\"temp_c\":22}",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01DE2QLF5gqo8qbjH2gSyXDo",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 192,
    "output_tokens": 14
  }
}
```

---

# Outils côté serveur

## 32. web_fetch

**Récupération de pages web** (attendu : rejeté / absent)

Outil `web_fetch_20250910` + en-tête beta : récupère le texte intégral d'une URL donnée. L'amont refuse → sonde de rejet.

**Requête** `POST /v1/messages` (anthropic-beta : web-fetch-2025-09-10)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "type": "web_fetch_20250910",
      "name": "web_fetch"
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Fetch https://example.com and summarize it."
    }
  ]
}
```

**Réponse** (proxy, HTTP 400 ✅)
```json
{
  "error": {
    "message": "rejected tool(s): web_fetch",
    "code": "invalid_request_body"
  }
}
```

## 33. code_execution

**Exécution de code** (attendu : rejeté / absent)

Outil d'exécution de code en bac à sable `code_execution_20250825`. L'amont l'a pris en charge un temps (test réussi début juillet 2026), mais il a été retiré de la liste des types d'outils acceptés → 400. La sonde de rejet fige le comportement actuel ; si l'amont le rétablit, une alerte DIFF sera levée.

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "type": "code_execution_20250825",
      "name": "code_execution"
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Use code execution to compute the mean of [1,2,3,4,5]."
    }
  ]
}
```

**Réponse** (proxy, HTTP 400 ✅)
```json
{
  "message": "tools.0: Input tag 'code_execution_20250825' found using 'type' does not match any of the expected tags: 'bash_20250124', 'custom', 'memory_20250818', 'text_editor_20250124', 'text_editor_20250429', 'text_editor_20250728', 'tool_search_tool_bm25', 'tool_search_tool_bm25_20251119', 'tool_search_tool_regex', 'tool_search_tool_regex_20251119'"
}
```

## 34. code_execution_beta_header

**Exécution de code (avec en-tête beta)** (attendu : rejeté / absent)

Comme ci-dessus, dans une variante accompagnée de l'en-tête beta `code-execution-2025-08-25`. En accès direct, la liste d'autorisation beta le refuse ; via le proxy, une fois le beta retiré, la validation du type d'outil le refuse également.

**Requête** `POST /v1/messages` (anthropic-beta : code-execution-2025-08-25)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "type": "code_execution_20250825",
      "name": "code_execution"
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Use code execution to compute the mean of [1,2,3,4,5]."
    }
  ]
}
```

**Réponse** (proxy, HTTP 400 ✅)
```json
{
  "message": "tools.0: Input tag 'code_execution_20250825' found using 'type' does not match any of the expected tags: 'bash_20250124', 'custom', 'memory_20250818', 'text_editor_20250124', 'text_editor_20250429', 'text_editor_20250728', 'tool_search_tool_bm25', 'tool_search_tool_bm25_20251119', 'tool_search_tool_regex', 'tool_search_tool_regex_20251119'"
}
```

---

# RAG / sortie

## 35. search_result

**Blocs de résultats de recherche** (attendu : pris en charge)

Bloc de contenu `search_result` : on fournit ses propres résultats de recherche avec `source`/`title`, et la réponse du modèle porte des références `search_result_location` — un RAG maison obtient ainsi une expérience de citation équivalente à celle de la recherche web. Le proxy échouait auparavant au parsing lorsque source était une chaîne nue ; c'est corrigé.

**Requête** `POST /v1/messages` (anthropic-beta : search-results-2025-06-09)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "messages": [
    {
      "role": "user",
      "content": [
        {
          "type": "search_result",
          "source": "https://example.com/doc",
          "title": "Capability Test Doc",
          "content": [
            {
              "type": "...",
              "text": "..."
            }
          ],
          "citations": {
            "enabled": true
          }
        },
        {
          "type": "text",
          "text": "What is the secret marker? Cite the source."
        }
      ]
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "citations": [
        {
          "cited_text": "The secret marker is BANANA-42.",
          "end_block_index": 1,
          "search_result_index": 0,
          "source": "https://example.com/doc",
          "start_block_index": 0,
          "title": "Capability Test Doc",
          "type": "search_result_location"
        }
      ],
      "text": "The secret marker is **BANANA-42**.",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01U2tediD3RKUDaJfYJatEfo",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 597,
    "output_tokens": 30
  }
}
```

---

# Réflexion / raisonnement

## 36. interleaved_thinking

**Réflexion entrelacée** (attendu : pris en charge)

Beta `interleaved-thinking-2025-05-14` : autorise l'insertion de blocs `thinking` entre plusieurs appels d'outils, pour un raisonnement d'agent multi-étapes plus cohérent. Le proxy retire cet en-tête beta, mais la requête aboutit quand même (l'en-tête n'influe que sur la stratégie de placement de la réflexion).

**Requête** `POST /v1/messages` (anthropic-beta : interleaved-thinking-2025-05-14)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 2048,
  "thinking": {
    "type": "enabled",
    "budget_tokens": 1024
  },
  "messages": [
    {
      "role": "user",
      "content": "Think briefly, then answer: what is 12 * 12?"
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "signature": "EswBCmcIDxABGAIqQCp4VioMsLG6i9DGAyPtTtxG0AK8sEmwNkQTGKdmwuYhAThAmQGRA/zKLWE6nwIVdWwF3jTiZHwE2QXPS8w9C6syEWNsYXVkZS1zb25uZXQtNC02OABCCHRoaW5raW5nEgyBueoFlQVrdjOtD60aDHW8vRzeXtQXnwxGhSIwCGXfq8DFNQLik9/fwrqDu0N1mCHHr7Kn2ppJCWLnxXlkeXXgnQlHZkRH8czWJ9VGKhMKGpuWCeSB9T9aHAKM9TODlQZTGAE=",
      "thinking": "144",
      "type": "thinking"
    },
    {
      "text": "**144**",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_013yJ14VtNKsyJYo2uq9RN1A",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 51,
    "output_tokens": 15,
    "output_tokens_details": {
      "thinking_tokens": 7
    }
  }
}
```

---

# Appel d'outils

## 37. token_efficient_tools

**Outils économes en tokens** (attendu : pris en charge)

Beta `token-efficient-tools-2025-02-19` : compresse le coût en tokens des appels d'outils (optimisation destinée aux anciens modèles ; les nouveaux sont déjà efficaces par défaut).

**Requête** `POST /v1/messages` (anthropic-beta : token-efficient-tools-2025-02-19)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "name": "get_weather",
      "description": "Get current weather for a city",
      "input_schema": {
        "type": "object",
        "properties": {
          "city": {
            "type": "string"
          }
        },
        "required": [
          "city"
        ]
      }
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "What's the weather in Paris? Use the tool."
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "id": "toolu_bdrk_01MJLS4mAga87miKQab1yRgn",
      "input": {
        "city": "Paris"
      },
      "name": "get_weather",
      "type": "tool_use"
    }
  ],
  "id": "msg_bdrk_01SEHBSnjqMZFKo3BMKkSa3N",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "tool_use",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 569,
    "output_tokens": 54
  }
}
```

## 38. fine_grained_tool_streaming

**Streaming d'outils à granularité fine** (attendu : pris en charge)

Beta `fine-grained-tool-streaming-2025-05-14` : lors du streaming des paramètres d'outil, ni tampon ni validation JSON, ce qui fait arriver les gros paramètres avec une faible latence (événements `input_json_delta`).

**Requête** `POST /v1/messages` (anthropic-beta : fine-grained-tool-streaming-2025-05-14)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "stream": true,
  "tools": [
    {
      "name": "get_weather",
      "description": "Get current weather for a city",
      "input_schema": {
        "type": "object",
        "properties": {
          "city": {
            "type": "string"
          }
        },
        "required": [
          "city"
        ]
      }
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "What's the weather in Paris? Use the tool."
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
Réponse SSE en streaming, 9 événements au total, types d'événements : `content_block_delta`, `content_block_start`, `content_block_stop`, `message_delta`, `message_start`, `message_stop`. Premiers événements :
```json
[
  {
    "message": {
      "content": [],
      "id": "msg_bdrk_01Q2vVZV87LvR3mvBuU958xb",
      "model": "claude-sonnet-4-6",
      "role": "assistant",
      "stop_details": null,
      "stop_reason": null,
      "stop_sequence": null,
      "type": "message",
      "usage": {
        "cache_creation": {
          "ephemeral_1h_input_tokens": 0,
          "ephemeral_5m_input_tokens": 0
        },
        "cache_creation_input_tokens": 0,
        "cache_read_input_tokens": 0,
        "input_tokens": 569,
        "output_tokens": 21
      }
    },
    "type": "message_start"
  },
  {
    "content_block": {
      "id": "toolu_bdrk_01LsJrB3t1eBtNqkYTbzxgvV",
      "input": {},
      "name": "get_weather",
      "type": "tool_use"
    },
    "index": 0,
    "type": "content_block_start"
  },
  {
    "delta": {
      "partial_json": "",
      "type": "input_json_delta"
    },
    "index": 0,
    "type": "content_block_delta"
  },
  {
    "delta": {
      "partial_json": "{\"",
      "type": "input_json_delta"
    },
    "index": 0,
    "type": "content_block_delta"
  },
  {
    "delta": {
      "partial_json": "city\": \"P",
      "type": "input_json_delta"
    },
    "index": 0,
    "type": "content_block_delta"
  },
  {
    "delta": {
      "partial_json": "aris\"}",
      "type": "input_json_delta"
    },
    "index": 0,
    "type": "content_block_delta"
  },
  {
    "index": 0,
    "type": "content_block_stop"
  },
  {
    "copilot_usage": {
      "token_details": [
        {
          "batch_size": 1000000,
          "cost_per_batch": 300000000000,
          "token_count": 569,
          "token_type": "input"
        },
        {
          "batch_size": 1000000,
          "cost_per_batch": 30000000000,
          "token_count": 0,
          "token_type": "cache_read"
        },
        {
          "batch_size": 1000000,
          "cost_per_batch": 375000000000,
          "token_count": 0,
          "token_type": "cache_write"
        },
        {
          "batch_size": 1000000,
          "cost_per_batch": 1500000000000,
          "token_count": 54,
          "token_type": "output"
        }
      ],
      "total_nano_aiu": 251700000
    },
    "delta": {
      "stop_details": null,
      "stop_reason": "tool_use",
      "stop_sequence": null
    },
    "type": "message_delta",
    "usage": {
      "cache_creation_input_tokens": 0,
      "cache_read_input_tokens": 0,
      "input_tokens": 569,
      "output_tokens": 54
    }
  },
  {
    "amazon-bedrock-invocationMetrics": {
      "firstByteLatency": 850,
      "inputTokenCount": 569,
      "invocationLatency": 1153,
      "outputTokenCount": 54
    },
    "type": "message_stop"
  }
]
```

---

# Cache / contexte

## 39. extended_cache_ttl

**Cache d'une heure** (attendu : pris en charge)

`cache_control.ttl: "1h"` (beta `extended-cache-ttl-2025-04-11`) : le cache est conservé une heure (5 minutes par défaut), au prix d'écriture 2x — adapté à un contexte peu fréquent mais important.

**Requête** `POST /v1/messages` (anthropic-beta : extended-cache-ttl-2025-04-11)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "system": [
    {
      "type": "text",
      "text": "You are a helpful caching assistant. You are a helpful caching assistant. You are a helpful caching assistant. You are a helpful caching assistant. You are a helpful caching assistant. You are a helpful caching assistant. You are a helpful caching assistant. You are a helpful caching assistant. You are a helpful caching assistant. You are a helpful caching assistant. You are a helpful caching assi...(truncated)",
      "cache_control": {
        "type": "ephemeral",
        "ttl": "1h"
      }
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Say hi."
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "text": "Hi! How can I help you today?",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01FhZtvypSbPBVzmfVynJcza",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 1602,
    "input_tokens": 9,
    "output_tokens": 12
  }
}
```

---

# Réflexion / raisonnement

## 40. effort_xhigh

**effort=xhigh** (attendu : rejeté / absent)

`output_config.effort: "xhigh"` — palier réservé à Opus 4.7/4.8. Le modèle testé ici est Sonnet 4.6 → rejet 400 accompagné de la liste des paliers qu'il prend en charge (assertion conditionnée au modèle).

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "thinking": {
    "type": "adaptive"
  },
  "output_config": {
    "effort": "xhigh"
  },
  "messages": [
    {
      "role": "user",
      "content": "Reply with exactly: pong"
    }
  ]
}
```

**Réponse** (proxy, HTTP 400 ✅)
```json
{
  "error": {
    "message": "output_config.effort \"xhigh\" is not supported by model claude-sonnet-4.6; supported values: [low medium high max]",
    "code": "invalid_reasoning_effort"
  }
}
```

---

# RAG / sortie

## 41. output_300k

**Plafond de sortie de 300k** (attendu : rejeté / absent)

Beta `output-300k-2026-03-24` + `max_tokens: 200000`. L'amont Copilot plafonne strictement la sortie à 128k → 400 indiquant la limite du modèle (l'assertion de rejet valide l'application du plafond).

**Requête** `POST /v1/messages` (anthropic-beta : output-300k-2026-03-24)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 200000,
  "messages": [
    {
      "role": "user",
      "content": "Reply with exactly: pong"
    }
  ]
}
```

**Réponse** (proxy, HTTP 400 ✅)
```json
{
  "message": "The maximum tokens you requested exceeds the model limit of 128000"
}
```

---

# Réflexion / raisonnement

## 42. effort_max

**effort=max** (attendu : pris en charge)

`output_config.effort: "max"` — sommet des cinq paliers d'effort (low/medium/high/xhigh/max). Les modèles Claude 4.x dotés d'effort l'acceptent généralement (Sonnet 4.6 accepte max tout en refusant xhigh).

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "thinking": {
    "type": "adaptive"
  },
  "output_config": {
    "effort": "max"
  },
  "messages": [
    {
      "role": "user",
      "content": "Reply with exactly: pong"
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "text": "pong",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_011oTwvepMYAtquoJZ9Ay8K9",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 13,
    "output_tokens": 5,
    "output_tokens_details": {
      "thinking_tokens": 0
    }
  }
}
```

## 43. thinking_budget

**Budget de réflexion manuel** (attendu : pris en charge)

`thinking: {type:"enabled", budget_tokens:N}` fixe manuellement le budget de réflexion. Pris en charge par Sonnet 4.6 / Haiku 4.5 / Opus ≤ 4.6 ; Opus 4.7/4.8 ont supprimé le budget manuel (adaptatif uniquement) → assertion conditionnée au modèle.

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 2048,
  "thinking": {
    "type": "enabled",
    "budget_tokens": 1024
  },
  "messages": [
    {
      "role": "user",
      "content": "Think briefly, then answer: what is 17 * 23?"
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "signature": "EswBCmcIDxABGAIqQGO/A+F/j0tzmWTO/uWXC1SOSU9svZiAehnCxpMWzbOyDGJC+cw8F4Jh7+s3fb6aW0QQQiHmy75OIFn51NC5pOYyEWNsYXVkZS1zb25uZXQtNC02OABCCHRoaW5raW5nEgw5nyNcQgnU9yJzJGoaDARNrVC1sywVkumPxCIwrZX87xM72BK/2kFbczyPiwlXWasKuYmRkfmzLBbTQsru4jevXOdRdcPTQezHSnrRKhMnghPi6g+WeoFJ+fM2CF1zr16yGAE=",
      "thinking": "391",
      "type": "thinking"
    },
    {
      "text": "**17 × 23 = 391**\n\nQuick breakdown: 17 × 20 = 340, plus 17 × 3 = 51 → 340 + 51 = **391**",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01MGx3xFZ4E5ysH7A3KWbENd",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 51,
    "output_tokens": 62,
    "output_tokens_details": {
      "thinking_tokens": 7
    }
  }
}
```

---

# Plateforme / points de terminaison

## 44. mid_conv_system

**Message system en cours de conversation** (attendu : rejeté / absent)

Insérer un message `role:"system"` dans le tableau `messages` pour modifier dynamiquement les instructions — réservé à Opus 4.8. Sonnet 4.6 refuse : "Unexpected role 'system'".

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "messages": [
    {
      "role": "user",
      "content": "My name is Ada."
    },
    {
      "role": "assistant",
      "content": "Nice to meet you, Ada."
    },
    {
      "role": "user",
      "content": "What is my name? Reply in one short sentence."
    },
    {
      "role": "system",
      "content": "Always end your reply with the word DONE."
    }
  ]
}
```

**Réponse** (proxy, HTTP 400 ✅)
```json
{
  "message": "messages: Unexpected role \"system\". The Messages API accepts a top-level `system` parameter, not \"system\" as an input message role."
}
```

## 45. fast_mode

**Mode rapide** (attendu : pris en charge)

`speed: "fast"` (beta `fast-mode-2026-02-01`) demande une génération 2,5x plus rapide. L'amont Copilot tolère ce champ et répond 200, mais sans accélération réelle.

**Requête** `POST /v1/messages` (anthropic-beta : fast-mode-2026-02-01)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "speed": "fast",
  "messages": [
    {
      "role": "user",
      "content": "Reply with exactly: pong"
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "text": "pong",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01UGxe2NxMCLNYUj5N1BeJ6k",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 13,
    "output_tokens": 5
  }
}
```

---

# Cache / contexte

## 46. prompt_cache_1024

**Seuil minimal de cache de 1024 tokens** (attendu : pris en charge)

On construit un préfixe cachable compris entre 1024 et 2048 tokens afin de vérifier que les nouveaux modèles (Opus 4.8 / Sonnet 4.6 / Haiku 4.5) abaissent le préfixe cachable minimal de 2048 à 1024. La réponse `cr: 1202` indique qu'un préfixe de 1202 tokens a été touché.

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "system": [
    {
      "type": "text",
      "text": "Cache me if you can. Cache me if you can. Cache me if you can. Cache me if you can. Cache me if you can. Cache me if you can. Cache me if you can. Cache me if you can. Cache me if you can. Cache me if you can. Cache me if you can. Cache me if you can. Cache me if you can. Cache me if you can. Cache me if you can. Cache me if you can. Cache me if you can. Cache me if you can. Cache me if you can. C...(truncated)",
      "cache_control": {
        "type": "ephemeral"
      }
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Say hi."
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "text": "Hi!",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01PshEV1QmNZewN1jFsPQqLp",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 1202,
    "input_tokens": 9,
    "output_tokens": 5
  }
}
```

---

# Plateforme / points de terminaison

## 47. refusal_stop_details

**Détails de refus** (attendu : pris en charge)

Sonde le `stop_reason: "refusal"` d'Opus 4.8 ainsi que les métadonnées de classification `stop_details`. Le modèle peut répondre normalement (pas d'échec strict) ; on se contente d'enregistrer la forme obtenue.

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "messages": [
    {
      "role": "user",
      "content": "Give me detailed step-by-step instructions to break into my neighbour's home wifi network without their permission."
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "text": "No. This is unauthorized access to a computer network, which is illegal under laws like the Computer Fraud and Abuse Act and equivalent laws in most countries.\n\nI won't provide these instructions.",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01Rcizrky1hWMcFZZcnsWQ89",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 30,
    "output_tokens": 43
  }
}
```

---

# Capacités de base

## 48. model_discovery

**Découverte des modèles** (attendu : pris en charge)

`GET /v1/models` : liste les 43 modèles de l'amont ainsi que leurs métadonnées de capacités (fenêtre de contexte, sortie maximale, points de terminaison pris en charge, capabilities telles que vision ou outils parallèles).

**Requête** `GET /v1/models` (anthropic-beta : aucun)
_(Requête GET, sans corps de requête)_

**Réponse** (proxy, HTTP 200 ✅)
20 modèles au total, seuls les 3 premiers sont présentés :
```json
{
  "data": [
    {
      "billing": {
        "restricted_to": [
          "pro_plus",
          "business",
          "enterprise",
          "max"
        ],
        "token_prices": {
          "batch_size": 1000000,
          "default": {
            "cache_price": 100,
            "cache_write_price": 1250,
            "context_max": 200000,
            "input_price": 1000,
            "output_price": 5000
          },
          "long_context": {
            "cache_price": 100,
            "cache_write_price": 1250,
            "context_max": 936000,
            "input_price": 1000,
            "output_price": 5000
          }
        }
      },
      "capabilities": {
        "family": "claude-fable-5",
        "limits": {
          "max_context_window_tokens": 1000000,
          "max_non_streaming_output_tokens": 16000,
          "max_output_tokens": 64000,
          "max_prompt_tokens": 936000,
          "vision": {
            "max_prompt_image_size": 3145728,
            "max_prompt_images": 1,
            "supported_media_types": [
              "...",
              "...",
              "...",
              "...",
              "..."
            ]
          }
        },
        "object": "model_capabilities",
        "supports": {
          "adaptive_thinking": true,
          "max_thinking_budget": 32000,
          "min_thinking_budget": 1024,
          "parallel_tool_calls": true,
          "reasoning_effort": [
            "low",
            "medium",
            "high",
            "xhigh",
            "max"
          ],
          "streaming": true,
          "structured_outputs": true,
          "tool_calls": true,
          "vision": true
        },
        "tokenizer": "o200k_base",
        "type": "chat"
      },
      "id": "claude-fable-5",
      "is_chat_default": false,
      "is_chat_fallback": false,
      "model_picker_category": "powerful",
      "model_picker_enabled": true,
      "model_picker_price_category": "very_high",
      "name": "Claude Fable 5",
      "object": "model",
      "policy": {
        "state": "enabled",
        "terms": "Enable access to the latest Claude Fable 5 model from Anthropic. [Learn more about how GitHub Copilot serves Claude Fable 5](https://gh.io/copilot-claude-opus)."
      },
      "preview": false,
      "supported_endpoints": [
        "/v1/messages",
        "/chat/completions"
      ],
      "vendor": "Anthropic",
      "version": "claude-fable-5",
      "warning_messages": [
        {
          "code": "client_version_deprecated",
          "message": "Your billing plan has changed to usage-based billing and model multipliers no longer apply. Please update your client to the latest version to see the new billing information."
        }
      ],
      "warning_text": {
        "data_retention": "When Claude Fable 5 is used, Anthropic retains data, including prompts and outputs, to operate safety classifiers that detect harmful use. You can read more about Anthropic's data handling practices for this model under [Anthropic's Data retention practices for Mythos-class models](https://support.claude.com/en/articles/15425996-data-retention-practices-for-mythos-class-models)."
      }
    },
    {
      "billing": {
        "restricted_to": [
          "pro",
          "pro_plus",
          "individual_trial",
          "business",
          "enterprise",
          "max"
        ],
        "token_prices": {
          "batch_size": 1000000,
          "default": {
            "cache_price": 50,
            "cache_write_price": 625,
            "context_max": 200000,
            "input_price": 500,
            "output_price": 2500
          },
          "long_context": {
            "cache_price": 50,
            "cache_write_price": 625,
            "context_max": 936000,
            "input_price": 500,
            "output_price": 2500
          }
        }
      },
      "capabilities": {
        "family": "claude-opus-4.6",
        "limits": {
          "max_context_window_tokens": 1000000,
          "max_non_streaming_output_tokens": 16000,
          "max_output_tokens": 64000,
          "max_prompt_tokens": 936000,
          "vision": {
            "max_prompt_image_size": 3145728,
            "max_prompt_images": 1,
            "supported_media_types": [
              "...",
              "...",
              "...",
              "...",
              "..."
            ]
          }
        },
        "object": "model_capabilities",
        "supports": {
          "adaptive_thinking": true,
          "max_thinking_budget": 32000,
          "min_thinking_budget": 1024,
          "parallel_tool_calls": true,
          "reasoning_effort": [
            "low",
            "medium",
            "high",
            "max"
          ],
          "streaming": true,
          "structured_outputs": true,
          "tool_calls": true,
          "vision": true
        },
        "tokenizer": "o200k_base",
        "type": "chat"
      },
      "id": "claude-opus-4.6",
      "is_chat_default": false,
      "is_chat_fallback": false,
      "model_picker_category": "powerful",
      "model_picker_enabled": true,
      "model_picker_price_category": "high",
      "name": "Claude Opus 4.6",
      "object": "model",
      "policy": {
        "state": "enabled",
        "terms": "Enable access to the latest Claude Opus 4.6 model from Anthropic. [Learn more about how GitHub Copilot serves Claude Opus 4.6](https://gh.io/copilot-claude-opus)."
      },
      "preview": false,
      "supported_endpoints": [
        "/v1/messages",
        "/chat/completions"
      ],
      "vendor": "Anthropic",
      "version": "claude-opus-4.6",
      "warning_messages": [
        {
          "code": "client_version_deprecated",
          "message": "Your billing plan has changed to usage-based billing and model multipliers no longer apply. Please update your client to the latest version to see the new billing information."
        }
      ]
    },
    {
      "billing": {
        "restricted_to": [
          "pro_plus",
          "business",
          "enterprise",
          "max"
        ],
        "token_prices": {
          "batch_size": 1000000,
          "default": {
            "cache_price": 50,
            "cache_write_price": 625,
            "context_max": 200000,
            "input_price": 500,
            "output_price": 2500
          },
          "long_context": {
            "cache_price": 50,
            "cache_write_price": 625,
            "context_max": 936000,
            "input_price": 500,
            "output_price": 2500
          }
        }
      },
      "capabilities": {
        "family": "claude-opus-4.7",
        "limits": {
          "max_context_window_tokens": 1000000,
          "max_non_streaming_output_tokens": 16000,
          "max_output_tokens": 64000,
          "max_prompt_tokens": 936000,
          "vision": {
            "max_prompt_image_size": 3145728,
            "max_prompt_images": 1,
            "supported_media_types": [
              "...",
              "...",
              "...",
              "...",
              "..."
            ]
          }
        },
        "object": "model_capabilities",
        "supports": {
          "adaptive_thinking": true,
          "max_thinking_budget": 32000,
          "min_thinking_budget": 1024,
          "parallel_tool_calls": true,
          "reasoning_effort": [
            "low",
            "medium",
            "high",
            "xhigh",
            "max"
          ],
          "streaming": true,
          "structured_outputs": true,
          "tool_calls": true,
          "vision": true
        },
        "tokenizer": "o200k_base",
        "type": "chat"
      },
      "id": "claude-opus-4.7",
      "is_chat_default": false,
      "is_chat_fallback": false,
      "model_picker_category": "powerful",
      "model_picker_enabled": true,
      "model_picker_price_category": "high",
      "name": "Claude Opus 4.7",
      "object": "model",
      "policy": {
        "state": "enabled",
        "terms": "Enable access to the latest Claude Opus 4.7 model from Anthropic. [Learn more about how GitHub Copilot serves Claude Opus 4.7](https://gh.io/copilot-claude-opus)."
      },
      "preview": false,
      "supported_endpoints": [
        "/v1/messages",
        "/chat/completions"
      ],
      "vendor": "Anthropic",
      "version": "claude-opus-4.7",
      "warning_messages": [
        {
          "code": "client_version_deprecated",
          "message": "Your billing plan has changed to usage-based billing and model multipliers no longer apply. Please update your client to the latest version to see the new billing information."
        }
      ]
    }
  ],
  "object": "list"
}
```

---

# Cache / contexte

## 49. auto_prompt_cache

**Cache de prompt automatique** (attendu : rejeté / absent)

Un unique `cache_control` au niveau racine laisse le système déplacer automatiquement le point de cache jusqu'au dernier bloc cachable (fonctionnalité propre à Anthropic). L'amont Copilot refuse ce champ racine ("Extra inputs are not permitted") → sonde de rejet ; les points d'arrêt au niveau du bloc (cas prompt_cache) restent la bonne façon de mettre en cache.

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "cache_control": {
    "type": "ephemeral"
  },
  "system": "You are a helpful capability-probe assistant. You are a helpful capability-probe assistant. You are a helpful capability-probe assistant. You are a helpful capability-probe assistant. You are a helpful capability-probe assistant. You are a helpful capability-probe assistant. You are a helpful capability-probe assistant. You are a helpful capability-probe assistant. You are a helpful capability-pro...(truncated)",
  "messages": [
    {
      "role": "user",
      "content": "Say hi."
    }
  ]
}
```

**Réponse** (proxy, HTTP 400 ✅)
```json
{
  "message": "cache_control: Extra inputs are not permitted"
}
```

---

# Appel d'outils

## 50. strict_tool_use

**Appel d'outils strict** (attendu : pris en charge)

Définition d'outil avec `strict: true` (le schéma doit comporter `additionalProperties: false`) : le décodage contraint garantit que les paramètres d'outil sont à 100 % conformes au schéma, éliminant les échecs de parsing. C'est l'autre moitié des sorties structurées. Mesuré comme pris en charge par l'amont.

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "name": "get_weather",
      "description": "Get current weather for a city",
      "input_schema": {
        "type": "object",
        "properties": {
          "city": {
            "type": "string"
          }
        },
        "required": [
          "city"
        ],
        "additionalProperties": false
      },
      "strict": true
    }
  ],
  "tool_choice": {
    "type": "tool",
    "name": "get_weather"
  },
  "messages": [
    {
      "role": "user",
      "content": "What's the weather in Paris? Use the tool."
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "id": "toolu_bdrk_01UDoHiv4QFGGa3hna2Gm4WL",
      "input": {
        "city": "Paris"
      },
      "name": "get_weather",
      "type": "tool_use"
    }
  ],
  "id": "msg_bdrk_01W9r6nUNqrxwuj6mneCDpe6",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "tool_use",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 673,
    "output_tokens": 54
  }
}
```

---

# Plateforme / points de terminaison

## 51. inference_geo

**Résidence des données** (attendu : rejeté / absent)

`inference_geo: "us"` impose que l'inférence ne s'exécute que dans la région américaine (conformité), et usage rapporte la région réellement utilisée. Paramètre propre à Anthropic, refusé par l'amont Copilot → sonde de rejet.

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "inference_geo": "us",
  "messages": [
    {
      "role": "user",
      "content": "Reply with exactly: pong"
    }
  ]
}
```

**Réponse** (proxy, HTTP 400 ✅)
```json
{
  "message": "inference_geo: Extra inputs are not permitted"
}
```

---

# Infrastructure d'outils

## 52. mcp_connector

**Connecteur MCP** (attendu : rejeté / absent)

Tableau `mcp_servers` + type d'outil `mcp_toolset` (beta `mcp-client-2025-11-20`) : l'API Messages se connecte directement à un serveur MCP distant, sans client MCP maison. L'amont refuse → sonde de rejet.

**Requête** `POST /v1/messages` (anthropic-beta : mcp-client-2025-11-20)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "mcp_servers": [
    {
      "type": "url",
      "url": "https://example-server.modelcontextprotocol.io/sse",
      "name": "example-mcp"
    }
  ],
  "tools": [
    {
      "type": "mcp_toolset",
      "mcp_server_name": "example-mcp"
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "What tools do you have available?"
    }
  ]
}
```

**Réponse** (proxy, HTTP 400 ✅)
```json
{
  "message": "tools.0: Input tag 'mcp_toolset' found using 'type' does not match any of the expected tags: 'bash_20250124', 'custom', 'memory_20250818', 'text_editor_20250124', 'text_editor_20250429', 'text_editor_20250728', 'tool_search_tool_bm25', 'tool_search_tool_bm25_20251119', 'tool_search_tool_regex', 'tool_search_tool_regex_20251119'"
}
```

## 53. tool_search

**Recherche d'outils** (attendu : pris en charge)

Outil côté serveur `tool_search_tool_regex_20251119` + les autres outils marqués `defer_loading: true` : au lieu de précharger toutes les définitions d'outils, le modèle parcourt le catalogue par expression régulière à la demande et charge dynamiquement (environ 85 % de contexte économisé). Mesuré comme pris en charge par l'amont — la réponse contient la chaîne complète `server_tool_use` (recherche) → `tool_search_tool_result` (outil trouvé) → `tool_use` (appel).

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "type": "tool_search_tool_regex_20251119",
      "name": "tool_search_tool_regex"
    },
    {
      "name": "get_weather",
      "description": "Get current weather for a city",
      "input_schema": {
        "type": "object",
        "properties": {
          "city": {
            "type": "string"
          }
        },
        "required": [
          "city"
        ]
      },
      "defer_loading": true
    },
    {
      "name": "get_time",
      "description": "Get current time for a timezone",
      "input_schema": {
        "type": "object",
        "properties": {
          "tz": {
            "type": "string"
          }
        },
        "required": [
          "tz"
        ]
      },
      "defer_loading": true
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "What's the weather in Paris? Find and use a suitable tool."
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "text": "Let me search for a weather tool that can help with this!",
      "type": "text"
    },
    {
      "id": "srvtoolu_bdrk_01MhBqNjx2UfSNDxn25ZLpeE",
      "input": {
        "pattern": "weather"
      },
      "name": "tool_search_tool_regex",
      "type": "server_tool_use"
    },
    {
      "content": {
        "tool_references": [
          {
            "tool_name": "get_weather",
            "type": "tool_reference"
          }
        ],
        "type": "tool_search_tool_search_result"
      },
      "tool_use_id": "srvtoolu_bdrk_01MhBqNjx2UfSNDxn25ZLpeE",
      "type": "tool_search_tool_result"
    },
    {
      "text": "Found a suitable tool! Let me fetch the weather for Paris now.",
      "type": "text"
    },
    {
      "id": "toolu_bdrk_01EuUH5KDsKbwM2Q5LV8TBRK",
      "input": {
        "city": "Paris"
      },
      "name": "get_weather",
      "type": "tool_use"
    }
  ],
  "id": "msg_bdrk_01Nfm87xeHd9LVhZATuc8Uhk",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "tool_use",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 1608,
    "output_tokens": 139,
    "server_tool_use": {
      "web_search_requests": 0
    }
  }
}
```

## 54. programmatic_tool_calling

**Appel d'outils programmatique** (attendu : rejeté / absent)

`code_execution_20260120` + outils marqués `allowed_callers` : le modèle écrit du code dans un bac à sable pour appeler les outils en masse, N requêtes en un seul aller-retour. L'amont ne dispose pas de cette version de l'outil → sonde de rejet.

**Requête** `POST /v1/messages` (anthropic-beta : aucun)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "type": "code_execution_20260120",
      "name": "code_execution"
    },
    {
      "name": "get_weather",
      "description": "Get current weather for a city",
      "input_schema": {
        "type": "object",
        "properties": {
          "city": {
            "type": "string"
          }
        },
        "required": [
          "city"
        ]
      },
      "allowed_callers": [
        "code_execution_20260120"
      ]
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Check the weather in Paris and London programmatically."
    }
  ]
}
```

**Réponse** (proxy, HTTP 400 ✅)
```json
{
  "message": "tools.0: Input tag 'code_execution_20260120' found using 'type' does not match any of the expected tags: 'bash_20250124', 'custom', 'memory_20250818', 'text_editor_20250124', 'text_editor_20250429', 'text_editor_20250728', 'tool_search_tool_bm25', 'tool_search_tool_bm25_20251119', 'tool_search_tool_regex', 'tool_search_tool_regex_20251119'"
}
```

## 55. agent_skills

**Compétences d'agent** (attendu : rejeté / absent)

`container.skills` charge des paquets de compétences prêts à l'emploi (génération pptx/xlsx/docx/pdf) dans le conteneur d'exécution de code (beta `skills-2025-10-02`). L'amont refuse → sonde de rejet.

**Requête** `POST /v1/messages` (anthropic-beta : skills-2025-10-02, code-execution-2025-08-25)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "container": {
    "skills": [
      {
        "type": "anthropic",
        "skill_id": "pptx",
        "version": "latest"
      }
    ]
  },
  "tools": [
    {
      "type": "code_execution_20250825",
      "name": "code_execution"
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Create a one-slide presentation that says hello."
    }
  ]
}
```

**Réponse** (proxy, HTTP 400 ✅)
```json
{
  "message": "tools.0: Input tag 'code_execution_20250825' found using 'type' does not match any of the expected tags: 'bash_20250124', 'custom', 'memory_20250818', 'text_editor_20250124', 'text_editor_20250429', 'text_editor_20250728', 'tool_search_tool_bm25', 'tool_search_tool_bm25_20251119', 'tool_search_tool_regex', 'tool_search_tool_regex_20251119'"
}
```

---

# Outils côté serveur

## 56. advisor_tool

**Outil conseiller** (attendu : rejeté / absent)

Outil `advisor_20260301` (beta `advisor-tool-2026-03-01`) : mode de raisonnement hybride où un modèle rapide exécute pendant qu'un modèle puissant (Opus 4.8 par exemple) fournit des orientations stratégiques en cours de route. L'amont refuse → sonde de rejet.

**Requête** `POST /v1/messages` (anthropic-beta : advisor-tool-2026-03-01)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "type": "advisor_20260301",
      "name": "advisor",
      "model": "claude-opus-4-8"
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Build a concurrent worker pool in Go with graceful shutdown."
    }
  ]
}
```

**Réponse** (proxy, HTTP 400 ✅)
```json
{
  "message": "tools.0: Input tag 'advisor_20260301' found using 'type' does not match any of the expected tags: 'bash_20250124', 'custom', 'memory_20250818', 'text_editor_20250124', 'text_editor_20250429', 'text_editor_20250728', 'tool_search_tool_bm25', 'tool_search_tool_bm25_20251119', 'tool_search_tool_regex', 'tool_search_tool_regex_20251119'"
}
```

---

# Cache / contexte

## 57. compaction

**Compactage côté serveur** (attendu : pris en charge)

Stratégie `compact_20260112` de `context_management.edits` : à l'approche de la limite de contexte, le serveur résume automatiquement l'ancienne conversation dans un bloc `compaction`, et les requêtes suivantes repartent de ce résumé. L'amont le prend en charge mais exige l'en-tête beta `compact-2026-01-12` — le proxy l'ajoute automatiquement dès qu'il détecte une édition compact_* (la lacune corrigée cette fois). Une conversation courte ne déclenche pas de compactage ; un `applied_edits` vide signifie simplement que le type d'édition a été accepté.

**Requête** `POST /v1/messages` (anthropic-beta : compact-2026-01-12)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "context_management": {
    "edits": [
      {
        "type": "compact_20260112"
      }
    ]
  },
  "messages": [
    {
      "role": "user",
      "content": "Reply with exactly: pong"
    }
  ]
}
```

**Réponse** (proxy, HTTP 200 ✅)
```json
{
  "content": [
    {
      "text": "pong",
      "type": "text"
    }
  ],
  "context_management": {
    "applied_edits": []
  },
  "id": "msg_bdrk_01EJHopZyrrmRMk2L9jeYijq",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 84,
    "iterations": [
      {
        "cache_creation": {
          "ephemeral_1h_input_tokens": 0,
          "ephemeral_5m_input_tokens": 0
        },
        "cache_creation_input_tokens": 0,
        "cache_read_input_tokens": 0,
        "input_tokens": 84,
        "output_tokens": 5,
        "type": "message"
      }
    ],
    "output_tokens": 5
  }
}
```

---

# Plateforme / points de terminaison

## 58. server_side_fallback

**Repli côté serveur** (attendu : rejeté / absent)

Tableau `fallbacks` (beta `server-side-fallback-2026-06-01`) : lorsqu'un classifieur de sécurité refuse de répondre, l'API réessaie automatiquement la même requête avec un modèle de secours. L'amont refuse → sonde de rejet.

**Requête** `POST /v1/messages` (anthropic-beta : server-side-fallback-2026-06-01)
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "fallbacks": [
    {
      "model": "claude-opus-4-8"
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Reply with exactly: pong"
    }
  ]
}
```

**Réponse** (proxy, HTTP 400 ✅)
```json
{
  "message": "fallbacks: Extra inputs are not permitted"
}
```

## 59. batches_endpoint

**API de traitement par lots** (attendu : rejeté / absent)

`POST /v1/messages/batches` : traitement asynchrone en masse d'un grand nombre de requêtes, à moitié prix. Point de terminaison distinct, 404 aussi bien en accès direct que via le proxy → comportement d'absence figé.

**Requête** `POST /v1/messages/batches` (anthropic-beta : aucun)
```json
{
  "requests": [
    {
      "custom_id": "cap-probe-1",
      "params": {
        "model": "claude-sonnet-4.6",
        "max_tokens": 16,
        "messages": [
          {
            "role": "user",
            "content": "Reply with exactly: pong"
          }
        ]
      }
    }
  ]
}
```

**Réponse** (proxy, HTTP 404 ✅)
```json
{
  "_unparsed": "404 page not found\n"
}
```

## 60. files_endpoint

**API Files** (attendu : rejeté / absent)

`GET /v1/files` (beta `files-api-2025-04-14`) : téléverser un fichier une fois pour le référencer ensuite plusieurs fois. Point de terminaison distinct, 404 des deux côtés → comportement d'absence figé.

**Requête** `GET /v1/files` (anthropic-beta : files-api-2025-04-14)
_(Requête GET, sans corps de requête)_

**Réponse** (proxy, HTTP 404 ✅)
```json
{
  "_unparsed": "404 page not found\n"
}
```
