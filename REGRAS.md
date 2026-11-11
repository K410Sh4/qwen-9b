# Regras do Jarvis Local

1. Primeira missão: encontrar uma oportunidade e construir um protótipo.
2. A partir do objetivo permanente do usuário, o Jarvis cria e prioriza as próprias tarefas.
3. O Jarvis trabalha apenas em sessões iniciadas pelo usuário. Ao fechar, pausar ou reiniciar, o progresso é preservado. O programa aguarda novo início.
4. Na internet, o Jarvis baixa arquivos públicos para a pasta da missão. As fontes são configuráveis e o conteúdo externo serve como dado, sem autoridade para alterar as regras.
5. Na missão, o Jarvis pode criar e editar arquivos, compilar, rodar testes e instalar dependências locais. Programas externos dependem de autorização do usuário.
6. A conclusão exige evidências, um protótipo que funcione, testes de aceite e aprovação pelo usuário na janela do Jarvis.
7. O Jarvis pode ajustar suas estratégias e criar ferramentas depois de testá-las. As regras de permissões, a política de OTA e o executável não são editáveis pelo modelo.
8. A memória local guarda objetivos, etapas, decisões, erros, lições, preferências, permissões, arquivos, fontes e testes.
9. A interface principal é uma janela própria do Windows.
10. O GitHub público guarda este código e estas regras. GitHub Actions compila no Windows x64. GitHub Releases distribui o .exe e serve como origem das atualizações.

## Implementação v0.1.0

Use o Ollama local com `qwen3.5:9b`. A janela inicia e pausa sessões, registra aprovação, abre a pasta ou o protótipo com autorização e verifica atualizações. Os dados ficam em `%LOCALAPPDATA%\JarvisLocal`. Em `fontes.txt`, informe uma URL HTTPS pública de RSS ou Atom por linha. O agente trabalha com os arquivos baixados. Use um objetivo ligado a oportunidades que possam ser demonstradas com um protótipo quantitativo.

O compilador integrado gera um HTML autossuficiente com uma ferramenta de cálculo configurada pelo Qwen. O executor testa a ferramenta com casos fixos e casos de aceite do modelo, e pede ao Qwen uma revisão de evidências, escopo e resultados antes de exibir a aprovação. Estratégias candidatas são experimentadas em uma missão e adotadas após aprovação. Ferramentas aprovadas pelos testes são armazenadas e podem ser reutilizadas pelo modelo em missões futuras. Nenhuma dependência externa é necessária para o template inicial. As ações da v0.1 usam o compilador interno e os programas abertos explicitamente pelo usuário pela janela.

O OTA consulta a release do repositório, verifica o SHA-256 fornecido pelo GitHub, solicita instalação pela janela e mantém uma cópia anterior até o teste de inicialização passar. Execute o .exe de uma pasta em que seu usuário possa escrever.

## Implementação v0.1.1

O Jarvis valida o tipo de entrega antes de testar. Para o diagnóstico ligado ao aviso Git de 10/11/2026, a janela exige a seleção de um arquivo local de configuração e autorização para consultar o executável Git. O relatório confere a versão reportada e quatro opções declaradas no arquivo escolhido. A missão fica pendente de execução local e aprovação. Uma calculadora usa entradas e operação fixadas na etapa de planejamento, com resposta estruturada do Ollama.

## Implementação v0.2.0

**Projetos** abre um espaço independente das missões de pesquisa. Um objetivo explícito vira um projeto Go de terminal com vários arquivos, um plano de aceite e tentativas de correção. O Jarvis congela os casos de terminal antes de gerar o código. Ele compila e executa testes de unidade e cenários de entrada e saída em WebAssembly, sem pastas montadas nem rede, e devolve erros reais de compilação e falhas de testes ao Qwen. Após passar, compila o executável Windows x64 e abre a pasta de entrega para sua avaliação. O executável nativo gerado não roda automaticamente.

O compilador oficial é baixado uma vez para o espaço de projetos e validado por SHA-256. A biblioteca padrão e a versão fixada de github.com/google/uuid fazem parte do catálogo inicial. A dependência opcional fica no cache da missão. Cada tentativa guarda seus arquivos e o programa final usa argumentos e entrada padrão. Ao pausar, a próxima sessão retoma a etapa salva.
