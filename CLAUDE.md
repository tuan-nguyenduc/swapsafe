# swapsafe — hướng dẫn cho Claude Code

## Dự án
CLI Go kiểm chứng Gateway API tái tạo đúng hành vi ingress-nginx.
Kế hoạch đầy đủ: docs/PLAN.md. Quyết định kiến trúc: docs/adr/.

## Lệnh
- make build | make test | make lint | make e2e | make golden
- Chạy một test: go test ./internal/differ -run TestName -race

## Quy tắc bắt buộc
- Chỉ làm đúng task được giao (ID dạng M1-T3). Không mở rộng phạm vi.
- Đọc mục liên quan trong docs/PLAN.md trước khi viết code.
- Tuân thủ quy tắc phụ thuộc package ở PLAN mục 4.
- Mọi hàm I/O nhận context.Context. Wrap lỗi bằng %w. Không panic ngoài main.
- Viết test trước hoặc cùng lúc với code; table-driven; chạy với -race.
- Rule mới phải có testdata: ingress.yaml, httproute-ok.yaml,
  httproute-bad.yaml, expected-findings.json.
- Không thêm dependency mới khi chưa hỏi.
- Không đổi định dạng JSON output trong pkg/api mà không cập nhật golden và CHANGELOG.
- GitHub Action trong workflow phải pin theo commit SHA.
- Không bao giờ ghi body response hoặc giá trị cookie vào log.

## Kết thúc mỗi task
1. make lint test xanh.
2. Cập nhật CHANGELOG mục Unreleased.
3. Cập nhật docs nếu thay đổi hành vi người dùng thấy.
4. Commit theo Conventional Commits, có sign-off (-s).
5. Tóm tắt: đã làm gì, chưa làm gì, rủi ro còn lại.
